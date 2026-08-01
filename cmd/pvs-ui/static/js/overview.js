'use strict';

import { fmt1, fmtKWh, setValue } from './display.js';
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
  setValue(solarEl, fmt1(c.solar_kw));
  setValue(loadEl,  fmt1(c.load_kw));
  prodCard.setAttribute('aria-label', 'Solar production: ' + fmt1(c.solar_kw) + ' kilowatts');
  usageCard.setAttribute('aria-label', 'Home usage: ' + fmt1(c.load_kw) + ' kilowatts');

  const netKW   = c.net_kw;
  const netEl   = document.getElementById('net-kw');
  const labelEl = document.getElementById('net-label');
  const cardEl  = document.getElementById('net-card');

  const exporting = netKW <= 0;
  setValue(netEl, fmt1(Math.abs(netKW)));
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

  setValue(solarEl, fmtKWh(s.solar_kwh));
  setValue(loadEl,  fmtKWh(s.load_kwh));
  setValue(avgEl,   fmt1(s.avg_solar_kw));

  solarEl.parentElement.setAttribute('aria-label', 'Energy produced: ' + fmtKWh(s.solar_kwh) + ' kilowatt-hours');
  loadEl.parentElement.setAttribute('aria-label', 'Energy consumed: ' + fmtKWh(s.load_kwh) + ' kilowatt-hours');
  avgEl.parentElement.setAttribute('aria-label', 'Average production: ' + fmt1(s.avg_solar_kw) + ' kilowatts');

  const net = s.net_kwh;
  setValue(netEl, fmtKWh(Math.abs(net)));
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

// Centered moving average of one field ('s' or 'l') over windowSec seconds.
// Each point is averaged with its neighbours within ±window/2. Null points are
// gap sentinels (see toSeriesPoints in pvs-api) and break the series into
// segments so we never average across a data gap.
function movingAvgField(series, key, windowSec) {
  const halfMs = (windowSec * 1000) / 2;
  const out = new Array(series.length);
  for (let i = 0; i < series.length; i++) {
    const p = series[i];
    if (p.s == null && p.l == null) { out[i] = null; continue; } // gap
    let sum = 0, n = 0;
    for (let j = i; j >= 0; j--) {                 // expand left within window
      const q = series[j];
      if (q.s == null && q.l == null) break;
      if (p.t - q.t > halfMs) break;
      sum += q[key]; n++;
    }
    for (let j = i + 1; j < series.length; j++) {  // expand right within window
      const q = series[j];
      if (q.s == null && q.l == null) break;
      if (q.t - p.t > halfMs) break;
      sum += q[key]; n++;
    }
    out[i] = sum / n;
  }
  return out;
}

// Smooth the production and usage lines independently. A window of 0 leaves
// that line raw. Returns the series unchanged when both are off.
export function smoothSeries(series, solarWin, loadWin) {
  if (!series || series.length === 0) return series;
  if (!solarWin && !loadWin) return series;
  const s = solarWin ? movingAvgField(series, 's', solarWin) : null;
  const l = loadWin  ? movingAvgField(series, 'l', loadWin)  : null;
  return series.map((p, i) => {
    if (p.s == null && p.l == null) return p; // preserve gap sentinel
    return { t: p.t, s: s ? s[i] : p.s, l: l ? l[i] : p.l };
  });
}

// Smoothing windows (seconds) offered for a given visible span. Mirrors the
// server's bucketSeconds tiers (pvs-api) so the smallest window is a few
// multiples of the data resolution — anything finer would be a no-op.
export function smoothingOptionsFor(spanSec) {
  const HOUR = 3600, DAY = 86400;
  let secs;
  if      (spanSec <= 2 * DAY)  secs = [600, 1800, 3600, 7200];
  else if (spanSec <= 7 * DAY)  secs = [2 * HOUR, 6 * HOUR, 12 * HOUR];
  else if (spanSec <= 90 * DAY) secs = [12 * HOUR, 2 * DAY, 7 * DAY];
  else                          secs = [2 * DAY, 7 * DAY, 30 * DAY];
  return [{ label: 'Off', sec: 0 }, ...secs.map(sec => ({ label: humanDur(sec), sec }))];
}

function humanDur(sec) {
  if (sec % 86400 === 0) { const d = sec / 86400; return d + ' day' + (d > 1 ? 's' : ''); }
  if (sec % 3600  === 0) return (sec / 3600) + ' hr';
  return (sec / 60) + ' min';
}

// Snap a previously-chosen window to the nearest option available for the new
// span (keeping Off as Off) so switching ranges never leaves a dangling value.
function snapWindow(win, opts) {
  if (!win) return 0;
  let best = win, diff = Infinity;
  for (const o of opts) {
    if (o.sec === 0) continue;
    const d = Math.abs(o.sec - win);
    if (d < diff) { diff = d; best = o.sec; }
  }
  return best;
}

function populateSmooth(sel, opts, value) {
  sel.innerHTML = '';
  for (const o of opts) {
    const el = document.createElement('option');
    el.value = String(o.sec);
    el.textContent = o.label;
    if (o.sec === value) el.selected = true;
    sel.appendChild(el);
  }
}

// Rebuild both smoothing dropdowns for the current span and snap the active
// windows onto the new option set. Called whenever a range is (re)loaded.
export function updateSmoothingOptions(since, until) {
  const solarSel = document.getElementById('smooth-solar');
  const loadSel  = document.getElementById('smooth-load');
  if (!solarSel || !loadSel) return;
  const opts = smoothingOptionsFor(Math.max(1, until - since));
  state.smoothingSolar = snapWindow(state.smoothingSolar, opts);
  state.smoothingLoad  = snapWindow(state.smoothingLoad, opts);
  populateSmooth(solarSel, opts, state.smoothingSolar);
  populateSmooth(loadSel,  opts, state.smoothingLoad);
}

// Last raw render inputs, kept so the smoothing control can re-render the
// current chart without re-fetching.
let _lastRender = null;

export function rerenderChart() {
  if (!_lastRender) return;
  const { series, rangeLabel, since, until, rangeName, events } = _lastRender;
  renderChart(series, rangeLabel, since, until, rangeName, events);
}

export function renderChart(series, rangeLabel, since, until, rangeName, events = []) {
  _lastRender = { series, rangeLabel, since, until, rangeName, events };
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

  const smoothed = smoothSeries(series, state.smoothingSolar, state.smoothingLoad);

  if (state.chart && rangeName && rangeName === state.chartRangeName) {
    const solar = smoothed.map(p => [p.t, p.s == null ? null : parseFloat(p.s.toFixed(3))]);
    const load  = smoothed.map(p => [p.t, p.l == null ? null : parseFloat(p.l.toFixed(3))]);
    state.chart.xAxis[0].setExtremes(since * 1000, until * 1000, false, false);
    state.chart.xAxis[0].update({ plotBands: buildPlotBands(events, since, until) }, false);
    state.chart.series[0].setData(solar, false, { duration: 400 });
    state.chart.series[1].setData(load,  true,  { duration: 400 });
    return;
  }

  if (state.chart) { try { state.chart.destroy(); } catch (_) {} state.chart = null; }
  toggleCreateEventButton(false);
  state.chart = Highcharts.chart('chart', buildChartOptions(smoothed, rangeLabel, since, until, events));
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
      // datetime-local values carry an explicit time, so use them as-is rather
      // than padding the end to end-of-day as the old date-only inputs required.
      const s = Math.floor(new Date(customSince) / 1000);
      const u = Math.floor(new Date(customUntil) / 1000);
      return { since: s, until: u, label: dateTimeRange(s * 1000, u * 1000) };
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
    // Reflect the actual charted window into the pickers. For Lifetime (or any
    // range reaching before data exists) this reveals when monitoring began —
    // an important clue — instead of a blank or epoch start.
    if (!state.isLive) syncPickers(chartSince, until);
    // Offer smoothing windows appropriate to the span now on screen.
    updateSmoothingOptions(chartSince, until);
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
  if (!state.isLive) syncPickers(since, until);
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
  syncPickers(newSince, clampedUntil);
  await fetchAndRender(newSince, clampedUntil, label, state.currentRange);
}

// ── Nav button state ──────────────────────────────────────────
let _prevBtn    = null;
let _nextBtn    = null;
let _navPeriod  = null;
let _customRow  = null;
let _customSinceEl = null;
let _customUntilEl = null;

// Format a Date as a datetime-local input value ("YYYY-MM-DDThh:mm") in local time.
function toDateTimeLocal(d) {
  const p = n => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`;
}

// Reflect the currently rendered range into the From/To pickers and reveal them.
// Called whenever a non-live range is loaded or shifted, so the pickers always
// show the active window and can be adjusted from there.
function syncPickers(sinceSec, untilSec) {
  if (!_customRow) return;
  if (_customSinceEl) _customSinceEl.value = sinceSec > 0 ? toDateTimeLocal(new Date(sinceSec * 1000)) : '';
  if (_customUntilEl) _customUntilEl.value = toDateTimeLocal(new Date(untilSec * 1000));
  _customRow.classList.add('visible');
}

function hidePickers() {
  if (_customRow) _customRow.classList.remove('visible');
}

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
export function initOverview() {
  const rangeSelect   = document.getElementById('range-select');
  _customRow          = document.getElementById('custom-range');
  _customSinceEl      = document.getElementById('custom-since');
  _customUntilEl      = document.getElementById('custom-until');
  const applyBtn      = document.getElementById('apply-custom');
  _prevBtn            = document.getElementById('prev-range');
  _nextBtn            = document.getElementById('next-range');
  _navPeriod          = document.getElementById('nav-period');

  rangeSelect.addEventListener('change', () => {
    const val = rangeSelect.value;
    if (val === 'live') {
      state.isLive = true;
      hidePickers();
      updateNavButtons();
      loadRange('today');
      return;
    }
    // Any fixed range: load it, then loadRange/syncPickers reveals the pickers
    // pre-filled with that range so it can be fine-tuned from there.
    state.isLive = false;
    if (val === 'custom') {
      state.currentRange = 'custom';
      // Seed the pickers if empty (first use), then render that window.
      if (!_customSinceEl.value || !_customUntilEl.value) {
        const now = new Date();
        const startOfToday = new Date(now.getFullYear(), now.getMonth(), now.getDate());
        if (!_customSinceEl.value) _customSinceEl.value = toDateTimeLocal(startOfToday);
        if (!_customUntilEl.value) _customUntilEl.value = toDateTimeLocal(now);
      }
      loadRange('custom', _customSinceEl.value, _customUntilEl.value);
    } else {
      state.currentRange = val;
      loadRange(val);
    }
  });

  applyBtn.addEventListener('click', () => {
    const since = _customSinceEl.value;
    const until = _customUntilEl.value;
    if (!since || !until) return;
    // Editing the pickers turns the active window into a custom range.
    state.currentRange = 'custom';
    rangeSelect.value = 'custom';
    loadRange('custom', since, until);
  });

  const solarSel = document.getElementById('smooth-solar');
  const loadSel  = document.getElementById('smooth-load');
  if (solarSel) solarSel.addEventListener('change', () => {
    state.smoothingSolar = parseInt(solarSel.value, 10) || 0;
    rerenderChart(); // re-render the already-fetched data, no refetch
  });
  if (loadSel) loadSel.addEventListener('change', () => {
    state.smoothingLoad = parseInt(loadSel.value, 10) || 0;
    rerenderChart();
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
