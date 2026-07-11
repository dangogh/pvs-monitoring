import { describe, it, expect, beforeEach, vi } from 'vitest';
import { resolveRange, buildChartOptions } from '../../cmd/pvs-ui/static/js/overview.js';

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
    expect(label).toBe('This Week');
  });

  it('this_month: since = first of month', () => {
    const { since, label } = resolveRange('this_month');
    expect(since).toBe(Math.floor(new Date(2024, 6, 1) / 1000));
    expect(label).toBe('This Month');
  });

  it('this_year: since = Jan 1', () => {
    const { since, label } = resolveRange('this_year');
    expect(since).toBe(Math.floor(new Date(2024, 0, 1) / 1000));
    expect(label).toBe('This Year');
  });

  it('past_24h: since = now - 86400', () => {
    const { since, until, label } = resolveRange('past_24h');
    expect(until - since).toBe(86400);
    expect(label).toBe('Past 24 Hours');
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
    expect(label).toBe('Past Year');
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
});
