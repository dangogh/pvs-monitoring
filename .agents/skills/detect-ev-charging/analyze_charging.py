#!/usr/bin/env python3
"""
Detect EV charging events from power data JSON.

Works on the raw series points (5-minute buckets from /api/data), not hourly
averages, so a charge that starts mid-hour is timed to the bucket where it
starts. A run of consecutive buckets above the threshold is one event.

Usage:
  python3 analyze_charging.py <api_response.json>
  python3 analyze_charging.py <api_response.json> --threshold 6 --min-duration 0.5
  python3 analyze_charging.py <api_response.json> --tz America/Los_Angeles

Times are the site's local time (--tz, default America/Los_Angeles), with DST
applied per timestamp. Generated event commands carry that zone's offset.
"""

import json
import statistics
import sys
from datetime import datetime, timedelta
from zoneinfo import ZoneInfo

DEFAULT_TZ = 'America/Los_Angeles'


def load_data(filepath):
    """Load API response JSON."""
    with open(filepath) as f:
        return json.load(f)


def bucket_ms(points):
    """Series cadence in milliseconds, taken from the data itself."""
    gaps = [b['t'] - a['t'] for a, b in zip(points, points[1:])]
    return statistics.median(gaps) if gaps else 300_000


def elevated_runs(points, min_load_kw, step_ms):
    """Yield (start, end) index pairs of contiguous elevated buckets; end is exclusive."""
    run_start = None
    for i, p in enumerate(points):
        contiguous = i > 0 and p['t'] - points[i - 1]['t'] == step_ms
        if p['l'] > min_load_kw:
            if run_start is not None and not contiguous:
                yield run_start, i
                run_start = None
            if run_start is None:
                run_start = i
        elif run_start is not None:
            yield run_start, i
            run_start = None
    if run_start is not None:
        yield run_start, len(points)


def analyze_charging(data, min_load_kw=6.0, min_duration_hours=1.0, tz=DEFAULT_TZ):
    """
    Analyze power series for EV charging events.

    Returns list of charging windows: {
        'date': '2026-11-15',
        'start': datetime (local),
        'end': datetime (local, exclusive),
        'duration_minutes': 90,
        'pre_charge_kw': 1.24,
        'avg_kw': 7.4,
        'peak_load_kw': 8.89,
        'post_charge_kw': 1.85,
    }
    """
    zone = ZoneInfo(tz)
    points = sorted(data.get('series', []), key=lambda p: p['t'])
    if not points:
        return []

    step_ms = bucket_ms(points)
    min_seconds = min_duration_hours * 3600

    events = []
    for a, b in elevated_runs(points, min_load_kw, step_ms):
        window = points[a:b]
        duration_s = (window[-1]['t'] - window[0]['t']) / 1000 + step_ms / 1000
        if duration_s < min_seconds:
            continue

        loads = [p['l'] for p in window]
        pre_load = points[a - 1]['l'] if a > 0 else loads[0]
        post_load = points[b]['l'] if b < len(points) else loads[-1]

        start = datetime.fromtimestamp(window[0]['t'] / 1000, tz=zone)
        end = datetime.fromtimestamp(window[-1]['t'] / 1000 + step_ms / 1000, tz=zone)

        events.append({
            'date': start.date().isoformat(),
            'start': start,
            'end': end,
            'duration_minutes': round(duration_s / 60),
            'pre_charge_kw': round(pre_load, 2),
            'avg_kw': round(sum(loads) / len(loads), 2),
            'peak_load_kw': round(max(loads), 2),
            'post_charge_kw': round(post_load, 2),
        })

    return events


def format_output(events):
    """Format charging events for display."""
    if not events:
        print("\nNo charging events detected.\n")
        return

    print("\n" + "=" * 70)
    print("DETECTED EV CHARGING EVENTS (site local time)")
    print("=" * 70 + "\n")

    for event in events:
        print(f"{event['date']}:")
        print(f"  Charge window:  {event['start']:%H:%M}—{event['end']:%H:%M} local")
        print(f"  Duration:       ~{event['duration_minutes']} minutes")
        print(f"  Pre-charge:     {event['pre_charge_kw']:.2f} kW")
        print(f"  Average load:   {event['avg_kw']:.2f} kW")
        print(f"  Peak load:      {event['peak_load_kw']:.2f} kW")
        print(f"  Post-charge:    {event['post_charge_kw']:.2f} kW")
        print()


def generate_event_commands(events):
    """Generate curl commands to create maintenance events."""
    if not events:
        return

    print("\n" + "=" * 70)
    print("CREATE MAINTENANCE EVENTS")
    print("=" * 70 + "\n")

    for event in events:
        print(f"curl -X POST 'http://solar.local/api/maintenance-events' \\")
        print(f"  -H 'Content-Type: application/json' \\")
        print(f"  -d '{{")
        print(f"    \"start_at\": \"{event['start'].isoformat()}\",")
        print(f"    \"end_at\": \"{event['end'].isoformat()}\",")
        print(f"    \"event_type\": \"ev_charging\",")
        print(f"    \"notes\": \"Buzzy EV charging ({event['peak_load_kw']} kW peak)\"")
        print(f"  }}'")
        print()


def main():
    if len(sys.argv) < 2:
        print("Usage: python3 analyze_charging.py <api_response.json> [--threshold N] [--min-duration H] [--tz ZONE]")
        sys.exit(1)

    filepath = sys.argv[1]
    threshold = 6.0
    min_duration = 1.0
    tz = DEFAULT_TZ

    # Parse optional arguments
    for i in range(2, len(sys.argv), 2):
        if sys.argv[i] == '--threshold' and i + 1 < len(sys.argv):
            threshold = float(sys.argv[i + 1])
        elif sys.argv[i] == '--min-duration' and i + 1 < len(sys.argv):
            min_duration = float(sys.argv[i + 1])
        elif sys.argv[i] == '--tz' and i + 1 < len(sys.argv):
            tz = sys.argv[i + 1]

    try:
        data = load_data(filepath)
    except (FileNotFoundError, json.JSONDecodeError) as e:
        print(f"Error loading {filepath}: {e}", file=sys.stderr)
        sys.exit(1)

    events = analyze_charging(data, min_load_kw=threshold, min_duration_hours=min_duration, tz=tz)
    format_output(events)
    generate_event_commands(events)


if __name__ == '__main__':
    main()
