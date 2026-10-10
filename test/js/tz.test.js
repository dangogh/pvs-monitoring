import { describe, it, expect } from 'vitest';
import { zonedParts, zonedTimeToUtcMs } from '../../cmd/pvs-ui/static/js/tz.js';

describe('zonedParts', () => {
  it('reads wall-clock parts in the given IANA zone', () => {
    // 2024-07-10T15:00:00Z is 08:00 PDT (UTC-7 in July).
    const p = zonedParts(new Date('2024-07-10T15:00:00Z'), 'America/Los_Angeles');
    expect(p).toMatchObject({ y: 2024, m: 6, d: 10, h: 8, min: 0, s: 0, dow: 3 }); // Wed
  });

  it('falls back to the runner\'s own timezone when undefined', () => {
    // Expected values come from the host's local clock, so this holds in
    // any runner timezone, not just UTC.
    const d = new Date('2024-07-10T15:00:00Z');
    const p = zonedParts(d, undefined);
    expect(p).toMatchObject({
      y: d.getFullYear(), m: d.getMonth(), d: d.getDate(),
      h: d.getHours(), min: d.getMinutes(), s: d.getSeconds(),
    });
  });

  it('handles a date that rolls to the next/previous day across the offset', () => {
    // 2024-07-10T02:00:00Z is still 2024-07-09 19:00 PDT.
    const p = zonedParts(new Date('2024-07-10T02:00:00Z'), 'America/Los_Angeles');
    expect(p).toMatchObject({ y: 2024, m: 6, d: 9, h: 19 });
  });
});

describe('zonedTimeToUtcMs', () => {
  it('is the inverse of zonedParts for a wall-clock time in a zone', () => {
    const ms = zonedTimeToUtcMs(2024, 6, 10, 0, 0, 0, 'America/Los_Angeles');
    expect(ms).toBe(new Date('2024-07-10T07:00:00Z').getTime());
  });

  it('round-trips through zonedParts', () => {
    const original = new Date('2024-11-20T12:34:56Z');
    const p = zonedParts(original, 'Europe/Stockholm');
    const back = zonedTimeToUtcMs(p.y, p.m, p.d, p.h, p.min, p.s, 'Europe/Stockholm');
    expect(back).toBe(original.getTime());
  });

  it('resolves correctly across a DST transition', () => {
    // Europe/Stockholm springs forward on 2024-03-31 at 02:00 → 03:00 CET→CEST.
    // Midnight the same day is still CET (UTC+1).
    const beforeDST = zonedTimeToUtcMs(2024, 2, 31, 0, 0, 0, 'Europe/Stockholm');
    expect(beforeDST).toBe(new Date('2024-03-30T23:00:00Z').getTime());
    // A week later it's CEST (UTC+2).
    const afterDST = zonedTimeToUtcMs(2024, 3, 7, 0, 0, 0, 'Europe/Stockholm');
    expect(afterDST).toBe(new Date('2024-04-06T22:00:00Z').getTime());
  });
});
