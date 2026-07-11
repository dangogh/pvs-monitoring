import { describe, it, expect, beforeEach } from 'vitest';
import { detailRow } from '../../cmd/pvs-ui/static/js/panels.js';
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
    expect(html).toContain('colspan="8"');
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
