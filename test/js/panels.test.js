import { describe, it, expect, beforeEach } from 'vitest';
import { detailRow, compareValues, sortPanels } from '../../cmd/pvs-ui/static/js/panels.js';
import { state } from '../../cmd/pvs-ui/static/js/state.js';

const makeDevice = (overrides = {}) => ({
  serial:          'SN001',
  state:           'working',
  state_descr:     'Working',
  power_kw:        1.234,
  today_kwh:       5.678,
  lifetime_kwh:    1000.0,
  current_a:       2.5,
  voltage_v:       240.1,
  freq_hz:         60.0,
  power_mppt1_kw:  1.3,
  voltage_mppt1_v: 380.0,
  current_mppt1_a: 3.4,
  temp_c:          42.1,
  ...overrides,
});

describe('detailRow', () => {
  it('returns a table row string', () => {
    const html = detailRow(makeDevice());
    expect(html).toMatch(/^<tr class="detail-row"/);
    expect(html).toContain('colspan="9"');
  });

  it('includes all expected field labels', () => {
    const html = detailRow(makeDevice());
    ['State', 'Power', 'Today', 'Current', 'Voltage (AC)', 'Frequency',
     'MPPT1 Power', 'MPPT1 Voltage', 'MPPT1 Current', 'Temperature', 'Lifetime']
      .forEach(label => expect(html).toContain(label));
  });

  it('renders formatted power value', () => {
    const html = detailRow(makeDevice({ power_kw: 1.234 }));
    expect(html).toContain('1.2');
  });

  it('renders null values as em-dash', () => {
    const html = detailRow(makeDevice({ power_kw: null }));
    expect(html).toContain('—');
  });

  it('includes unit spans', () => {
    const html = detailRow(makeDevice());
    expect(html).toContain('<span class="detail-unit">kW</span>');
    expect(html).toContain('<span class="detail-unit">°C</span>');
  });
});

describe('state.expandedSerials', () => {
  beforeEach(() => { state.expandedSerials.clear(); });

  it('starts empty', () => {
    expect(state.expandedSerials.size).toBe(0);
  });

  it('can add and check serials', () => {
    state.expandedSerials.add('SN001');
    expect(state.expandedSerials.has('SN001')).toBe(true);
  });

  it('can delete serials', () => {
    state.expandedSerials.add('SN001');
    state.expandedSerials.delete('SN001');
    expect(state.expandedSerials.has('SN001')).toBe(false);
  });
});

describe('compareValues', () => {
  it('compares numbers by subtraction, ascending', () => {
    expect(compareValues(1, 2, false)).toBeLessThan(0);
    expect(compareValues(2, 1, false)).toBeGreaterThan(0);
    expect(compareValues(1, 1, false)).toBe(0);
  });

  it('negates the comparison when desc is true', () => {
    expect(compareValues(1, 2, true)).toBeGreaterThan(0);
    expect(compareValues(2, 1, true)).toBeLessThan(0);
  });

  it('compares strings numeric-aware, so "Panel 10" sorts after "Panel 2"', () => {
    expect(compareValues('Panel 2', 'Panel 10', false)).toBeLessThan(0);
    expect(compareValues('Panel 10', 'Panel 2', false)).toBeGreaterThan(0);
  });
});

describe('sortPanels', () => {
  const cols = {
    power_kw: d => d.power_kw,
    label:    d => d.label,
    // Mirrors renderPanels' own rel accessor: non-finite ratios sort as
    // Infinity rather than corrupting the comparator with NaN.
    rel:      d => Number.isFinite(d.ratio) ? d.ratio : Infinity,
  };

  it('sorts numerically ascending', () => {
    const data = [{ power_kw: 3 }, { power_kw: 1 }, { power_kw: 2 }];
    expect(sortPanels(data, cols, 'power_kw', true).map(d => d.power_kw)).toEqual([1, 2, 3]);
  });

  it('sorts numerically descending', () => {
    const data = [{ power_kw: 3 }, { power_kw: 1 }, { power_kw: 2 }];
    expect(sortPanels(data, cols, 'power_kw', false).map(d => d.power_kw)).toEqual([3, 2, 1]);
  });

  it('sorts strings numeric-aware', () => {
    const data = [{ label: 'Panel 10' }, { label: 'Panel 2' }, { label: 'Panel 1' }];
    expect(sortPanels(data, cols, 'label', true).map(d => d.label))
      .toEqual(['Panel 1', 'Panel 2', 'Panel 10']);
  });

  it('does not mutate the input array', () => {
    const data = [{ power_kw: 3 }, { power_kw: 1 }];
    sortPanels(data, cols, 'power_kw', true);
    expect(data.map(d => d.power_kw)).toEqual([3, 1]);
  });

  it('sorts a panel with a non-finite ratio to the end, ascending', () => {
    const data = [{ ratio: NaN }, { ratio: 0.9 }, { ratio: 0.5 }];
    expect(sortPanels(data, cols, 'rel', true).map(d => d.ratio)).toEqual([0.5, 0.9, NaN]);
  });
});
