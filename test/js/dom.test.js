import { describe, it, expect, beforeEach, vi } from 'vitest';
import { updateCurrent, updateSummary } from '../../cmd/pvs-ui/static/js/overview.js';

// jsdom environment is configured in vitest.config.js

function setupDOM() {
  document.body.innerHTML = `
    <span id="solar-kw"></span>
    <span id="load-kw"></span>
    <div class="stat-card production" aria-label=""></div>
    <div class="stat-card usage" aria-label=""></div>
    <span id="net-kw"></span>
    <span id="net-label"></span>
    <div id="net-card" class="stat-card"></div>
    <span id="now-timestamp"></span>
    <span id="now-dot"></span>
    <span id="status"></span>
    <span id="period-label"></span>
    <div class="summary-card"><span id="sum-solar"></span><span class="summary-unit"></span></div>
    <div class="summary-card"><span id="sum-load"></span><span class="summary-unit"></span></div>
    <div class="summary-card"><span id="sum-avg"></span><span class="summary-unit"></span></div>
    <div class="summary-card"><span id="sum-net"></span><span class="summary-unit">kWh exported</span></div>
  `;
}

beforeEach(() => {
  setupDOM();
  vi.useFakeTimers();
});

describe('updateCurrent', () => {
  it('does nothing when called with null', () => {
    expect(() => updateCurrent(null)).not.toThrow();
  });

  it('sets solar and load text (after animation delay)', () => {
    const now = new Date();
    updateCurrent({ solar_kw: 3.5, load_kw: 2.1, net_kw: -1.4, updated_at: now.toISOString() });
    vi.advanceTimersByTime(200);
    expect(document.getElementById('solar-kw').textContent).toBe('3.5');
    expect(document.getElementById('load-kw').textContent).toBe('2.1');
  });

  it('shows Exporting when net_kw is negative', () => {
    const now = new Date();
    updateCurrent({ solar_kw: 3.5, load_kw: 2.1, net_kw: -1.4, updated_at: now.toISOString() });
    vi.advanceTimersByTime(200);
    expect(document.getElementById('net-label').textContent).toBe('Exporting');
    expect(document.getElementById('net-card').className).toContain('net-export');
  });

  it('shows Importing when net_kw is positive', () => {
    const now = new Date();
    updateCurrent({ solar_kw: 1.0, load_kw: 2.5, net_kw: 1.5, updated_at: now.toISOString() });
    vi.advanceTimersByTime(200);
    expect(document.getElementById('net-label').textContent).toBe('Importing');
    expect(document.getElementById('net-card').className).toContain('net-import');
  });

  it('marks reading as stale when age > 120s', () => {
    const staleTime = new Date(Date.now() - 200_000).toISOString();
    updateCurrent({ solar_kw: 0, load_kw: 0, net_kw: 0, updated_at: staleTime });
    expect(document.getElementById('now-dot').classList.contains('stale')).toBe(true);
  });

  it('does not mark as stale when age < 120s', () => {
    const freshTime = new Date(Date.now() - 10_000).toISOString();
    updateCurrent({ solar_kw: 0, load_kw: 0, net_kw: 0, updated_at: freshTime });
    expect(document.getElementById('now-dot').classList.contains('stale')).toBe(false);
  });

  it('sets status to "just now" for very recent readings', () => {
    const now = new Date().toISOString();
    updateCurrent({ solar_kw: 0, load_kw: 0, net_kw: 0, updated_at: now });
    expect(document.getElementById('status').textContent).toBe('Last reading just now');
  });

  it('sets status with seconds for readings < 60s old', () => {
    const t = new Date(Date.now() - 30_000).toISOString();
    updateCurrent({ solar_kw: 0, load_kw: 0, net_kw: 0, updated_at: t });
    expect(document.getElementById('status').textContent).toMatch(/Last reading \d+s ago/);
  });
});

describe('updateSummary', () => {
  const summary = {
    solar_kwh: 12.345,
    load_kwh:  8.901,
    avg_solar_kw: 2.3,
    net_kwh: -3.444,
  };

  it('sets period label', () => {
    updateSummary(summary, 'Today');
    vi.advanceTimersByTime(200);
    expect(document.getElementById('period-label').textContent).toBe('Today');
  });

  it('does not crash if label is omitted', () => {
    expect(() => updateSummary(summary)).not.toThrow();
  });

  it('sets solar kWh after animation', () => {
    updateSummary(summary, 'Today');
    vi.advanceTimersByTime(200);
    expect(document.getElementById('sum-solar').textContent).toBe('12.35');
  });

  it('shows "kWh exported" for negative net', () => {
    updateSummary({ ...summary, net_kwh: -3.0 }, 'Today');
    vi.advanceTimersByTime(200);
    expect(document.querySelector('#sum-net').parentElement.querySelector('.summary-unit').textContent)
      .toBe('kWh exported');
  });

  it('shows "kWh imported" for positive net', () => {
    updateSummary({ ...summary, net_kwh: 2.0 }, 'Today');
    vi.advanceTimersByTime(200);
    expect(document.querySelector('#sum-net').parentElement.querySelector('.summary-unit').textContent)
      .toBe('kWh imported');
  });
});
