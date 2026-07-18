import { describe, it, expect, beforeEach, vi } from 'vitest';
import { resolveRange, buildChartOptions, computeShift } from '../../cmd/pvs-ui/static/js/overview.js';

// Fix "now" so date-math tests are deterministic.
// 2024-07-10 is a Wednesday (day-of-week 3).
const FIXED_NOW = new Date('2024-07-10T15:00:00');
beforeEach(() => { vi.setSystemTime(FIXED_NOW); });

describe('resolveRange', () => {
  it('today: since = midnight, until = now', () => {
    const { since, until, label } = resolveRange('today');
    const midnight = Math.floor(new Date(2024, 6, 10) / 1000);
    expect(since).toBe(midnight);
    expect(until).toBeCloseTo(Math.floor(FIXED_NOW / 1000), -1);
    expect(label).toBe('Today');
  });

  it('this_week: since = Sunday midnight', () => {
    const { since, label } = resolveRange('this_week');
    // 2024-07-10 is Wed; Sunday of this week is 2024-07-07
    const sunday = Math.floor(new Date(2024, 6, 7) / 1000);
    expect(since).toBe(sunday);
    expect(label).toContain('–'); // date range format
  });

  it('this_month: since = first of month', () => {
    const { since, label } = resolveRange('this_month');
    expect(since).toBe(Math.floor(new Date(2024, 6, 1) / 1000));
    expect(label).toContain('2024'); // month name + year
  });

  it('this_year: since = Jan 1', () => {
    const { since, label } = resolveRange('this_year');
    expect(since).toBe(Math.floor(new Date(2024, 0, 1) / 1000));
    expect(label).toBe('2024');
  });

  it('past_24h: since = now - 86400', () => {
    const { since, until, label } = resolveRange('past_24h');
    expect(until - since).toBe(86400);
    expect(label).toContain('–'); // date range format
  });

  it('past_7d: window is exactly 7 days', () => {
    const { since, until } = resolveRange('past_7d');
    expect(until - since).toBe(7 * 86400);
  });

  it('past_30d: window is exactly 30 days', () => {
    const { since, until } = resolveRange('past_30d');
    expect(until - since).toBe(30 * 86400);
  });

  it('past_year: since = same day last year', () => {
    const { since, label } = resolveRange('past_year');
    expect(since).toBe(Math.floor(new Date(2023, 6, 10) / 1000));
    expect(label).toContain('–'); // date range format
  });

  it('lifetime: since = 0', () => {
    const { since, label } = resolveRange('lifetime');
    expect(since).toBe(0);
    expect(label).toBe('Lifetime');
  });

  it('custom: calculates from provided dates', () => {
    const { since, until, label } = resolveRange('custom', '2024-06-01', '2024-06-30');
    expect(since).toBe(Math.floor(new Date('2024-06-01') / 1000));
    expect(until).toBe(Math.floor(new Date('2024-06-30') / 1000) + 86399);
    expect(label).toBe('2024-06-01 – 2024-06-30');
  });

  it('unknown range falls back to today', () => {
    const { label } = resolveRange('bogus');
    expect(label).toBe('Today');
  });
});

describe('buildChartOptions', () => {
  const series = [
    { t: 1000000, s: 1.2345, l: 0.987 },
    { t: 1000060, s: null,   l: 1.0   },
  ];

  it('maps solar data and rounds to 3 decimals', () => {
    const opts = buildChartOptions(series, 'Today', 1000000, 1001000);
    expect(opts.series[0].data[0]).toEqual([1000000, 1.234]);
    expect(opts.series[0].data[1]).toEqual([1000060, null]);
  });

  it('maps load data', () => {
    const opts = buildChartOptions(series, 'Today', 1000000, 1001000);
    expect(opts.series[1].data[0]).toEqual([1000000, 0.987]);
    expect(opts.series[1].data[1]).toEqual([1000060, 1.0]);
  });

  it('sets xAxis min/max from since/until (ms)', () => {
    const opts = buildChartOptions(series, 'Today', 1000, 2000);
    expect(opts.xAxis.min).toBe(1000 * 1000);
    expect(opts.xAxis.max).toBe(2000 * 1000);
  });

  it('production series is area type in amber', () => {
    const opts = buildChartOptions(series, 'Today', 0, 1);
    expect(opts.series[0].type).toBe('area');
    expect(opts.series[0].color).toBe('#f59e0b');
  });

  it('usage series is line type in blue', () => {
    const opts = buildChartOptions(series, 'Today', 0, 1);
    expect(opts.series[1].type).toBe('line');
    expect(opts.series[1].color).toBe('#60a5fa');
  });

  it('includes accessibility description with range label', () => {
    const opts = buildChartOptions(series, 'This Week', 0, 1);
    expect(opts.accessibility.description).toContain('This Week');
  });

  it('disables exporting', () => {
    const opts = buildChartOptions(series, 'Today', 0, 1);
    expect(opts.exporting.enabled).toBe(false);
  });

  it('selection handler shows the create-event button and allows default zoom', () => {
    document.body.innerHTML = '<button id="create-event-btn" hidden></button>';
    const opts = buildChartOptions(series, 'Today', 0, 1);
    const result = opts.chart.events.selection({ xAxis: [{ min: 500, max: 800 }] });
    expect(result).toBe(true);
    expect(document.getElementById('create-event-btn').hidden).toBe(false);
  });

  it('selection handler hides the create-event button on zoom reset', () => {
    document.body.innerHTML = '<button id="create-event-btn"></button>';
    const opts = buildChartOptions(series, 'Today', 0, 1);
    opts.chart.events.selection({ resetSelection: true });
    expect(document.getElementById('create-event-btn').hidden).toBe(true);
  });
});

// ── computeShift ─────────────────────────────────────────────
// Anchored to 2024-07-10 15:00 UTC (Wednesday).
// since = midnight 2024-07-10 local = new Date(2024,6,10) / 1000
const DAY = 86400;
const WEEK = 7 * DAY;

describe('computeShift — today/past_24h (1-day window)', () => {
  const since = Math.floor(new Date(2024, 6, 10) / 1000); // midnight Jul 10
  const until = since + DAY;

  it('prev shifts back 1 day and snaps until to end of that day', () => {
    const r = computeShift('today', since, until, -1);
    expect(r.since).toBe(since - DAY);
    expect(r.until).toBe(since - 1);
  });

  it('next shifts forward 1 day and snaps until to end of that day', () => {
    const r = computeShift('today', since, until, +1);
    expect(r.since).toBe(since + DAY);
    expect(r.until).toBe(since + 2 * DAY - 1);
  });

  it('past_24h behaves identically', () => {
    const r = computeShift('past_24h', since, until, -1);
    expect(r.since).toBe(since - DAY);
  });
});

describe('computeShift — this_week/past_7d (7-day window)', () => {
  const since = Math.floor(new Date(2024, 6, 7) / 1000); // Sunday Jul 7
  const until = Math.floor(new Date(2024, 6, 10, 15) / 1000);

  it('prev shifts back 7 days and snaps until to end of that week', () => {
    const r = computeShift('this_week', since, until, -1);
    expect(r.since).toBe(since - WEEK);
    expect(r.until).toBe(since - 1);
  });

  it('past_7d behaves identically', () => {
    const r = computeShift('past_7d', since, until, -1);
    expect(r.since).toBe(since - WEEK);
  });
});

describe('computeShift — this_month (calendar month)', () => {
  const since = Math.floor(new Date(2024, 6, 1) / 1000); // Jul 1
  const until = Math.floor(new Date(2024, 6, 10, 15) / 1000);

  it('prev lands on June 1', () => {
    const r = computeShift('this_month', since, until, -1);
    expect(r.since).toBe(Math.floor(new Date(2024, 5, 1) / 1000));
  });

  it('next lands on August 1', () => {
    const r = computeShift('this_month', since, until, +1);
    expect(r.since).toBe(Math.floor(new Date(2024, 7, 1) / 1000));
  });

  it('crossing year boundary: Jan → Dec previous year', () => {
    const janSince = Math.floor(new Date(2024, 0, 1) / 1000);
    const r = computeShift('this_month', janSince, until, -1);
    expect(r.since).toBe(Math.floor(new Date(2023, 11, 1) / 1000));
  });
});

describe('computeShift — this_year (calendar year)', () => {
  const since = Math.floor(new Date(2024, 0, 1) / 1000);
  const until = Math.floor(new Date(2024, 6, 10, 15) / 1000);

  it('prev lands on 2023-01-01', () => {
    const r = computeShift('this_year', since, until, -1);
    expect(r.since).toBe(Math.floor(new Date(2023, 0, 1) / 1000));
  });

  it('next lands on 2025-01-01', () => {
    const r = computeShift('this_year', since, until, +1);
    expect(r.since).toBe(Math.floor(new Date(2025, 0, 1) / 1000));
  });
});

describe('computeShift — past_30d', () => {
  const since = Math.floor(new Date(2024, 6, 10, 15) / 1000) - 30 * DAY;
  const until = Math.floor(new Date(2024, 6, 10, 15) / 1000);

  it('prev shifts back 30 days', () => {
    const r = computeShift('past_30d', since, until, -1);
    expect(r.since).toBe(since - 30 * DAY);
    expect(r.until).toBe(until - 30 * DAY);
  });
});

describe('computeShift — custom/default (shift by duration)', () => {
  const since = 1000000;
  const until = 1000000 + 3 * DAY; // 3-day custom window

  it('prev shifts back by duration', () => {
    const r = computeShift('custom', since, until, -1);
    expect(r.since).toBe(since - 3 * DAY);
    expect(r.until).toBe(until - 3 * DAY);
  });

  it('next shifts forward by duration', () => {
    const r = computeShift('custom', since, until, +1);
    expect(r.since).toBe(since + 3 * DAY);
    expect(r.until).toBe(until + 3 * DAY);
  });
});
