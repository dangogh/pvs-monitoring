import { describe, it, expect } from 'vitest';
import {
  formatRatio, LOW_LIGHT_KW, median, peerMedians, peerRatio,
  ratioTitle, underperformers, UNDERPERFORM_RATIO, UNGROUPED,
} from '../../cmd/pvs-ui/static/js/peers.js';

const p = (serial, power_kw) => ({ serial, power_kw });

// Two peer groups shaded at opposite ends of the day: 'am' is producing well,
// 'pm' is in shade. This is the case a fleet-wide median gets wrong.
const twoGroups = [
  p('A1', 1.0), p('A2', 1.1), p('A3', 0.9),
  p('B1', 0.4), p('B2', 0.5), p('B3', 0.45),
];
const groups = { A1: 'am', A2: 'am', A3: 'am', B1: 'pm', B2: 'pm', B3: 'pm' };

describe('median', () => {
  it('returns the middle value for odd counts', () => {
    expect(median([3, 1, 2])).toBe(2);
  });

  it('averages the two middle values for even counts', () => {
    expect(median([1, 2, 3, 4])).toBe(2.5);
  });

  it('ignores non-finite values', () => {
    expect(median([1, NaN, 2, undefined, 3, null])).toBe(2);
  });

  it('returns NaN for an empty list', () => {
    expect(median([])).toBeNaN();
  });

  it('does not mutate its input', () => {
    const input = [3, 1, 2];
    median(input);
    expect(input).toEqual([3, 1, 2]);
  });
});

describe('peerMedians', () => {
  it('computes a median per group and for the fleet', () => {
    const { medians, fleet } = peerMedians(twoGroups, groups);
    expect(medians.am).toBeCloseTo(1.0);
    expect(medians.pm).toBeCloseTo(0.45);
    expect(fleet).toBeCloseTo(0.7);   // mean of the middle pair, 0.5 and 0.9
  });

  it('pools panels with no group into ungrouped', () => {
    const { medians } = peerMedians([p('X', 1), p('Y', 3)], {});
    expect(medians[UNGROUPED]).toBe(2);
  });

  it('handles a partial grouping, leaving the rest pooled', () => {
    const { medians } = peerMedians(twoGroups, { A1: 'am', A2: 'am', A3: 'am' });
    expect(medians.am).toBeCloseTo(1.0);
    expect(medians[UNGROUPED]).toBeCloseTo(0.45);
  });
});

describe('peerRatio', () => {
  it('compares a panel against its own group, not the fleet', () => {
    const ctx = peerMedians(twoGroups, groups);
    // B1 is 0.4 kW: 89% of its shaded group, but only 57% of the fleet.
    // Against the fleet median it would look like a fault; it is not one.
    const r = peerRatio(p('B1', 0.4), ctx, groups);
    expect(r.group).toBe('pm');
    expect(r.ratio).toBeCloseTo(0.889, 2);
    expect(r.dark).toBe(false);
  });

  it('flags a genuine underperformer within its group', () => {
    const panels = [...twoGroups, p('B4', 0.1)];
    const ctx = peerMedians(panels, { ...groups, B4: 'pm' });
    const r = peerRatio(p('B4', 0.1), ctx, { ...groups, B4: 'pm' });
    expect(r.ratio).toBeLessThan(UNDERPERFORM_RATIO);
  });

  it('reports dark when the fleet median is below the low-light floor', () => {
    const dim = [p('A1', 0.01), p('A2', 0.02), p('A3', 0.015)];
    const ctx = peerMedians(dim, {});
    const r = peerRatio(dim[0], ctx, {});
    expect(r.dark).toBe(true);
    expect(r.ratio).toBeNaN();
  });

  it('treats the low-light floor as inclusive', () => {
    const at = [p('A1', LOW_LIGHT_KW), p('A2', LOW_LIGHT_KW), p('A3', LOW_LIGHT_KW)];
    const ctx = peerMedians(at, {});
    expect(peerRatio(at[0], ctx, {}).dark).toBe(false);
  });

  it('reports dark when the panel\'s own group median is zero', () => {
    // The whole group is out (a tripped branch) while the array still produces:
    // there is no meaningful denominator, so no ratio is claimed.
    const panels = [p('A1', 1.0), p('A2', 1.1), p('A3', 0.9),
                    p('B1', 0), p('B2', 0), p('B3', 0)];
    const ctx = peerMedians(panels, groups);
    const r = peerRatio(p('B1', 0), ctx, groups);
    expect(r.dark).toBe(true);
    expect(r.ratio).toBeNaN();
  });
});

describe('underperformers', () => {
  it('returns only panels below the threshold', () => {
    const panels = [...twoGroups, p('B4', 0.05)];
    const g = { ...groups, B4: 'pm' };
    expect(underperformers(panels, g).map(d => d.serial)).toEqual(['B4']);
  });

  it('returns nothing when it is too dark to compare', () => {
    const dim = [p('A1', 0.01), p('A2', 0.02), p('A3', 0.0)];
    expect(underperformers(dim, {})).toEqual([]);
  });

  it('does not flag a whole shaded group as underperforming', () => {
    // Every 'pm' panel is at ~45% of the fleet median but ~100% of its peers.
    expect(underperformers(twoGroups, groups)).toEqual([]);
  });

  it('accepts a caller-supplied threshold', () => {
    expect(underperformers(twoGroups, groups, 0.99).length).toBeGreaterThan(0);
  });
});

describe('formatRatio and ratioTitle', () => {
  it('renders a percentage', () => {
    expect(formatRatio({ ratio: 0.876, dark: false })).toBe('88%');
  });

  it('renders an em dash rather than a number when dark', () => {
    expect(formatRatio({ ratio: NaN, dark: true })).toBe('—');
  });

  it('names the peer group in the title', () => {
    expect(ratioTitle({ ratio: 0.5, dark: false, group: 'pm' })).toBe('50% of pm median');
  });

  it('explains the low-light state in words', () => {
    expect(ratioTitle({ ratio: NaN, dark: true, group: 'pm' })).toBe('too dark to compare');
  });
});
