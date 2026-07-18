'use strict';

import { fmt1, fmtKWh, setValueAnimated, flashCard } from './display.js';
import { state } from './state.js';
import { prefillEventRange } from './events.js';

// ── Chart-selection → create-event button ───────────────────────
let pendingSelection = null;

function toggleCreateEventButton(show) {
  const btn = document.getElementById('create-event-btn');
  if (btn) btn.hidden = !show;
  if (!show) pendingSelection = null;
}

// ── Current reading ───────────────────────────────────────────
export function updateCurrent(c) {
  if (!c) return;
  const solarEl   = document.getElementById('solar-kw');
  const loadEl    = document.getElementById('load-kw');
  const prodCard  = document.querySelector('.stat-card.production');
  const usageCard = document.querySelector('.stat-card.usage');
  setValueAnimated(solarEl, fmt1(c.solar_kw)); flashCard(prodCard);
  setValueAnimated(loadEl,  fmt1(c.load_kw));  flashCard(usageCard);
  prodCard.setAttribute('aria-label', 'Solar production: ' + fmt1(c.solar_kw) + ' kilowatts');
  usageCard.setAttribute('aria-label', 'Home usage: ' + fmt1(c.load_kw) + ' kilowatts');

  const netKW   = c.net_kw;
  const netEl   = document.getElementById('net-kw');
  const labelEl = document.getElementById('net-label');
  const cardEl  = document.getElementById('net-card');

  const exporting = netKW <= 0;
  setValueAnimated(netEl, fmt1(Math.abs(netKW))); flashCard(cardEl);
  labelEl.textContent = exporting ? 'Exporting' : 'Importing';
  cardEl.className    = 'stat-card ' + (exporting ? 'net-export' : 'net-import');
  cardEl.setAttribute('aria-label', (exporting ? 'Exporting to grid: ' : 'Importing from grid: ') + fmt1(Math.abs(netKW)) + ' kilowatts');

  const updatedAt = new Date(c.updated_at);
  const age = Math.round((Date.now() - updatedAt.getTime()) / 1000);
  const stale = age > 120;
  const timeStr = updatedAt.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
  document.getElementById('now-timestamp').textContent = timeStr;
  document.getElementById('now-dot').classList.toggle('stale', stale);

  let ageStr;
  if      (age < 5)     ageStr = 'just now';
  else if (age < 60)    ageStr = age + 's ago';
  else if (age < 3600)  ageStr = Math.floor(age / 60) + 'm ago';
  else if (age < 86400) ageStr = Math.floor(age / 3600) + 'h ago';
  else                  ageStr = Math.floor(age / 86400) + 'd ago';
  document.getElementById('status').textContent = 'Last reading ' + ageStr;
}

// ── Summary cards ─────────────────────────────────────────────
export function updateSummary(s, label) {
  const periodLabel = document.getElementById('period-label');
  if (label && periodLabel) periodLabel.textContent = label;

  const solarEl = document.getElementById('sum-solar');
  const loadEl  = document.getElementById('sum-load');
  const avgEl   = document.getElementById('sum-avg');
  const netEl   = document.getElementById('sum-net');

  setValueAnimated(solarEl, fmtKWh(s.solar_kwh)); flashCard(solarEl.parentElement);
  setValueAnimated(loadEl,  fmtKWh(s.load_kwh));  flashCard(loadEl.parentElement);
  setValueAnimated(avgEl,   fmt1(s.avg_solar_kw)); flashCard(avgEl.parentElement);

  solarEl.parentElement.setAttribute('aria-label', 'Energy produced: ' + fmtKWh(s.solar_kwh) + ' kilowatt-hours');
  loadEl.parentElement.setAttribute('aria-label', 'Energy consumed: ' + fmtKWh(s.load_kwh) + ' kilowatt-hours');
  avgEl.parentElement.setAttribute('aria-label', 'Average production: ' + fmt1(s.avg_solar_kw) + ' kilowatts');

  const net = s.net_kwh;
  setValueAnimated(netEl, fmtKWh(Math.abs(net))); flashCard(netEl.parentElement);
  const netUnit = net < 0 ? 'kWh exported' : 'kWh imported';
  netEl.parentElement.querySelector('.summary-unit').textContent = netUnit;
  netEl.parentElement.setAttribute('aria-label', (net < 0 ? 'Net energy exported: ' : 'Net energy imported: ') + fmtKWh(Math.abs(net)) + ' kilowatt-hours');
}

// ── Maintenance event plot bands ──────────────────────────────
const EVENT_COLORS = {
  panel_cleaning: 'rgba(52, 211, 153, 0.12)',
  hvac_outage:    'rgba(248, 113, 113, 0.12)',
};
const EVENT_LABELS = {
  panel_cleaning: 'Panel Cleaning',
  hvac_outage:    'HVAC Outage',
};

function eventColor(type) {
  return EVENT_COLORS[type] || 'rgba(148, 163, 184, 0.10)';
}

function eventLabel(type) {
  return EVENT_LABELS[type] || type.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase());
}

export function buildPlotBands(events, since, until) {
  const sinceMs = since * 1000;
  const untilMs = until * 1000;
  const bands = [];
  for (const e of events) {
    const start = new Date(e.start_at).getTime();
    // No end_at means a point-in-time event; widen slightly so it's visible on the chart.
    const end = e.end_at ? new Date(e.end_at).getTime() : start + 3600_000;
    if (end < sinceMs || start > untilMs) continue;
    bands.push({
      from:  Math.max(start, sinceMs),
      to:    Math.min(end,   untilMs),
      color: eventColor(e.event_type),
      label: {
        text:  eventLabel(e.event_type),
        style: { color: '#94a3b8', fontSize: '0.7rem' },
        align: 'center',
        verticalAlign: 'top',
        y: 16,
      },
      zIndex: 1,
    });
  }
  return bands;
}

// ── Chart ─────────────────────────────────────────────────────
export function buildChartOptions(series, rangeLabel, since, until, events = []) {
  const solar = series.map(p => [p.t, p.s == null ? null : parseFloat(p.s.toFixed(3))]);
  const load  = series.map(p => [p.t, p.l == null ? null : parseFloat(p.l.toFixed(3))]);

  return {
    time: { useUTC: false },
    chart: {
      backgroundColor: 'transparent',
      style: { fontFamily: 'inherit', color: '#f1f5f9' },
      animation: false,
      zoomType: 'x',
      events: {
        selection: function (event) {
          if (event.resetSelection) {
            toggleCreateEventButton(false);
            return true;
          }
          const axis = event.xAxis && event.xAxis[0];
          if (axis) {
            pendingSelection = { min: axis.min, max: axis.max };
            toggleCreateEventButton(true);
          }
          return true;
        },
      },
      resetZoomButton: {
        theme: {
          fill: '#1e293b',
          stroke: '#334155',
          style: { color: '#f1f5f9' },
          states: { hover: { fill: '#334155' } },
        },
      },
    },
    title: { text: null },
    credits: { enabled: false },
    legend: {
      itemStyle: { color: '#f1f5f9', fontWeight: 'normal' },
      itemHoverStyle: { color: '#fff' },
    },
    xAxis: {
      type: 'datetime',
      min: since * 1000,
      max: until * 1000,
      lineColor: '#334155',
      tickColor: '#334155',
      labels: { style: { color: '#94a3b8' } },
      plotBands: buildPlotBands(events, since, until),
    },
    yAxis: {
      title: { text: 'kW', style: { color: '#94a3b8' } },
      gridLineColor: '#334155',
      labels: { style: { color: '#94a3b8' }, format: '{value:.1f}' },
      min: 0,
    },
    tooltip: {
      shared: true,
      backgroundColor: '#1e293b',
      borderColor: '#334155',
      style: { color: '#f1f5f9' },
      valueDecimals: 2,
      valueSuffix: ' kW',
    },
    plotOptions: {
      area: { marker: { enabled: false } },
      line: { marker: { enabled: false } },
    },
    series: [
      {
        name: 'Production',
        type: 'area',
        data: solar,
        color: '#f59e0b',
        fillOpacity: 0.15,
        lineWidth: 2,
      },
      {
        name: 'Usage',
        type: 'line',
        data: load,
        color: '#60a5fa',
        lineWidth: 2,
      },
    ],
    accessibility: {
      enabled: true,
      description: 'Solar production and home energy usage over time for ' + rangeLabel + '.',
      point: { valueDescriptionFormat: '{xDescription}: production {point.y:.2f} kW' },
      series: { descriptionFormat: '{seriesDescription}' },
      screenReaderSection: {
        beforeChartFormat:
          '<h3>Solar production and usage — {rangeLabel}</h3>' +
          '<div>Line chart with two series: solar production in amber, home usage in blue.</div>' +
          '<div>{chartLongdesc}</div>',
      },
      keyboardNavigation: { enabled: true },
    },
    exporting: { enabled: false },
  };
}

export function renderChart(series, rangeLabel, since, until, rangeName, events = []) {
  const noData  = document.getElementById('no-data');
  const chartEl = document.getElementById('chart');

  if (!series || series.length === 0) {
    chartEl.style.display = 'none';
    noData.style.display  = 'flex';
    if (state.chart) { state.chart.destroy(); state.chart = null; }
    state.chartRangeName = null;
    return;
  }

  chartEl.style.display = 'block';
  noData.style.display  = 'none';

  if (state.chart && rangeName && rangeName === state.chartRangeName) {
    const solar = series.map(p => [p.t, p.s == null ? null : parseFloat(p.s.toFixed(3))]);
    const load  = series.map(p => [p.t, p.l == null ? null : parseFloat(p.l.toFixed(3))]);
    state.chart.xAxis[0].setExtremes(since * 1000, until * 1000, false, false);
    state.chart.xAxis[0].update({ plotBands: buildPlotBands(events, since, until) }, false);
    state.chart.series[0].setData(solar, false, { duration: 400 });
    state.chart.series[1].setData(load,  true,  { duration: 400 });
    return;
  }

  if (state.chart) { try { state.chart.destroy(); } catch (_) {} state.chart = null; }
  toggleCreateEventButton(false);
  state.chart = Highcharts.chart('chart', buildChartOptions(series, rangeLabel, since, until, events));
  state.chartRangeName = rangeName || null;
}

// ── Range resolution ──────────────────────────────────────────
function fmtDate(d) {
  return d.toLocaleDateString([], { month: 'short', day: 'numeric', year: 'numeric' });
}

function fmtDateTime(d) {
  return d.toLocaleDateString([], { month: 'short', day: 'numeric', year: 'numeric' }) +
    ' ' + d.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' });
}

function dateRange(sinceMs, untilMs) {
  return fmtDate(new Date(sinceMs)) + ' – ' + fmtDate(new Date(untilMs));
}

function dateTimeRange(sinceMs, untilMs) {
  return fmtDateTime(new Date(sinceMs)) + ' – ' + fmtDateTime(new Date(untilMs));
}

export function resolveRange(name, customSince, customUntil) {
  const now   = new Date();
  const y     = now.getFullYear();
  const m     = now.getMonth();
  const d     = now.getDate();
  const today = new Date(y, m, d);
  const until = Math.floor(now / 1000);

  switch (name) {
    case 'today':
      return { since: Math.floor(today / 1000), until, label: 'Today' };
    case 'this_week': {
      const dow = now.getDay();
      const s   = Math.floor(new Date(y, m, d - dow) / 1000);
      return { since: s, until, label: dateRange(s * 1000, until * 1000) };
    }
    case 'this_month':
      return { since: Math.floor(new Date(y, m, 1) / 1000), until, label: now.toLocaleDateString([], { month: 'long', year: 'numeric' }) };
    case 'this_year':
      return { since: Math.floor(new Date(y, 0, 1) / 1000), until, label: String(y) };
    case 'past_24h': {
      const s = until - 86400;
      return { since: s, until, label: dateRange(s * 1000, until * 1000) };
    }
    case 'past_7d': {
      const s = until - 7 * 86400;
      return { since: s, until, label: dateRange(s * 1000, until * 1000) };
    }
    case 'past_30d': {
      const s = until - 30 * 86400;
      return { since: s, until, label: dateRange(s * 1000, until * 1000) };
    }
    case 'past_year': {
      const s = Math.floor(new Date(y - 1, m, d) / 1000);
      return { since: s, until, label: dateRange(s * 1000, until * 1000) };
    }
    case 'lifetime':
      return { since: 0, until, label: 'Lifetime' };
    case 'custom': {
      const s = Math.floor(new Date(customSince) / 1000);
      const u = Math.floor(new Date(customUntil) / 1000) + 86399;
      return { since: s, until: u, label: customSince + ' – ' + customUntil };
    }
    default:
      return { since: Math.floor(today / 1000), until, label: 'Today' };
  }
}

// ── Shift label ───────────────────────────────────────────────
function shiftLabel(name, since, until) {
  const s = new Date(since * 1000);
  const u = new Date(until * 1000);
  const fmtDate = (d) => d.toLocaleDateString([], { month: 'short', day: 'numeric', year: 'numeric' });
  const range   = () => fmtDate(s) + ' – ' + fmtDate(u);
  switch (name) {
    case 'today':
      return s.toLocaleDateString([], { weekday: 'short', month: 'short', day: 'numeric', year: 'numeric' });
    case 'this_week':
      return fmtDate(s) + ' – ' + fmtDate(u);
    case 'this_month':
      return s.toLocaleDateString([], { month: 'long', year: 'numeric' });
    case 'this_year':
      return String(s.getFullYear());
    default:
      return range();
  }
}

// ── Fetch and render ──────────────────────────────────────────
export async function fetchAndRender(since, until, label, rangeName) {
  const container = document.getElementById('chart-container');
  const overlay = document.createElement('div');
  overlay.className = 'loading-overlay';
  overlay.innerHTML = '<div class="spinner"></div>';
  container.appendChild(overlay);
  try {
    const url = state.apiBase + '/api/data?since=' + since + '&until=' + until;
    const resp = await fetch(url);
    if (!resp.ok) throw new Error('HTTP ' + resp.status);
    const data = await resp.json();

    updateCurrent(data.current);
    updateSummary(data.summary, label);
    const chartSince = data.earliest_at ? Math.max(since, Math.floor(new Date(data.earliest_at) / 1000)) : since;
    updateNavButtons(dateTimeRange(chartSince * 1000, until * 1000));
    renderChart(data.series, label, chartSince, until, rangeName, state.maintenanceEvents);
  } catch (e) {
    document.getElementById('status').textContent = 'Error: ' + e.message;
  } finally {
    overlay.remove();
  }
}

// ── Overview data loading ─────────────────────────────────────
export async function loadRange(name, customSince, customUntil) {
  const { since, until, label } = resolveRange(name, customSince, customUntil);
  state.lastSince = since;
  state.lastUntil = until;
  updateNavButtons(label);
  await fetchAndRender(since, until, label, name);
}

export async function refreshCurrent() {
  try {
    const resp = await fetch(state.apiBase + '/api/current');
    if (!resp.ok) return;
    updateCurrent(await resp.json());
  } catch (_) {}
}

// ── Prev/next navigation ──────────────────────────────────────
export function computeShift(name, since, until, direction) {
  const d = direction;
  const sinceDate = new Date(since * 1000);

  switch (name) {
    case 'today': {
      const newSince = since + d * 86400;
      return { since: newSince, until: newSince + 86400 - 1 };
    }
    case 'past_24h':
      return { since: since + d * 86400, until: until + d * 86400 };
    case 'this_week': {
      const newSince = since + d * 7 * 86400;
      return { since: newSince, until: newSince + 7 * 86400 - 1 };
    }
    case 'past_7d':
      return { since: since + d * 7 * 86400, until: until + d * 7 * 86400 };
    case 'this_month': {
      const s = new Date(sinceDate);
      s.setMonth(s.getMonth() + d);
      const e = new Date(s);
      e.setMonth(e.getMonth() + 1);
      return { since: Math.floor(s / 1000), until: Math.floor(e / 1000) - 1 };
    }
    case 'past_30d':
      return { since: since + d * 30 * 86400, until: until + d * 30 * 86400 };
    case 'this_year': {
      const s = new Date(sinceDate);
      s.setFullYear(s.getFullYear() + d);
      const e = new Date(s);
      e.setFullYear(e.getFullYear() + 1);
      return { since: Math.floor(s / 1000), until: Math.floor(e / 1000) - 1 };
    }
    case 'past_year':
      return { since: since + d * 365 * 86400, until: until + d * 365 * 86400 };
    default: {
      const dur = until - since;
      return { since: since + d * dur, until: until + d * dur };
    }
  }
}

export async function shiftRange(direction) {
  if (state.lastSince == null || state.currentRange === 'lifetime') return;
  const { since: newSince, until: newUntil } = computeShift(
    state.currentRange, state.lastSince, state.lastUntil, direction
  );
  const now = Math.floor(Date.now() / 1000);
  if (newSince >= now) return;
  const clampedUntil = Math.min(newUntil, now);
  state.lastSince = newSince;
  state.lastUntil = clampedUntil;
  const label = shiftLabel(state.currentRange, newSince, clampedUntil);
  updateNavButtons(label);
  await fetchAndRender(newSince, clampedUntil, label, state.currentRange);
}

// ── Nav button state ──────────────────────────────────────────
let _prevBtn    = null;
let _nextBtn    = null;
let _navPeriod  = null;

export function updateNavButtons(label) {
  if (!_prevBtn) return;
  const isLifetime = state.currentRange === 'lifetime';
  const atPresent  = state.lastUntil != null && state.lastUntil >= Math.floor(Date.now() / 1000) - 60;
  _prevBtn.disabled = isLifetime;
  _nextBtn.disabled = isLifetime || atPresent;
  const hidden = state.isLive;
  _prevBtn.hidden = hidden;
  _nextBtn.hidden = hidden;
  if (_navPeriod) _navPeriod.hidden = hidden;
  if (label != null && _navPeriod) _navPeriod.textContent = label;
}

// ── Range select UI ───────────────────────────────────────────
function todayStr() { return new Date().toISOString().slice(0, 10); }

export function initOverview() {
  const rangeSelect   = document.getElementById('range-select');
  const customRow     = document.getElementById('custom-range');
  const customSinceEl = document.getElementById('custom-since');
  const customUntilEl = document.getElementById('custom-until');
  const applyBtn      = document.getElementById('apply-custom');
  _prevBtn            = document.getElementById('prev-range');
  _nextBtn            = document.getElementById('next-range');
  _navPeriod          = document.getElementById('nav-period');

  rangeSelect.addEventListener('change', () => {
    const val = rangeSelect.value;
    if (val === 'live') {
      state.isLive = true;
      customRow.classList.remove('visible');
      updateNavButtons();
      loadRange('today');
    } else if (val === 'custom') {
      state.isLive = false;
      state.currentRange = 'custom';
      if (!customSinceEl.value) customSinceEl.value = todayStr();
      if (!customUntilEl.value) customUntilEl.value = todayStr();
      customRow.classList.add('visible');
      updateNavButtons();
    } else {
      state.isLive = false;
      customRow.classList.remove('visible');
      state.currentRange = val;
      loadRange(state.currentRange);
    }
  });

  applyBtn.addEventListener('click', () => {
    const since = customSinceEl.value;
    const until = customUntilEl.value;
    if (!since || !until) return;
    loadRange('custom', since, until);
  });

  _prevBtn.addEventListener('click', () => shiftRange(-1));
  _nextBtn.addEventListener('click', () => shiftRange(+1));

  const createEventBtn = document.getElementById('create-event-btn');
  createEventBtn?.addEventListener('click', () => {
    if (!pendingSelection) return;
    const { min, max } = pendingSelection;
    document.getElementById('btn-events')?.click();
    prefillEventRange(min, max);
    toggleCreateEventButton(false);
  });

  updateNavButtons();
}
