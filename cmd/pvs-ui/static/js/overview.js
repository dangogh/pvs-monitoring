'use strict';

import { fmt1, fmtKWh, setValueAnimated, flashCard } from './display.js';
import { state } from './state.js';

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
  if (label) document.getElementById('period-label').textContent = label;

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

// ── Chart ─────────────────────────────────────────────────────
export function buildChartOptions(series, rangeLabel, since, until) {
  const solar = series.map(p => [p.t, p.s == null ? null : parseFloat(p.s.toFixed(3))]);
  const load  = series.map(p => [p.t, p.l == null ? null : parseFloat(p.l.toFixed(3))]);

  return {
    time: { useUTC: false },
    chart: {
      backgroundColor: 'transparent',
      style: { fontFamily: 'inherit', color: '#f1f5f9' },
      animation: false,
      zoomType: 'x',
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

export function renderChart(series, rangeLabel, since, until, rangeName) {
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
    state.chart.series[0].setData(solar, false, { duration: 400 });
    state.chart.series[1].setData(load,  true,  { duration: 400 });
    return;
  }

  if (state.chart) { try { state.chart.destroy(); } catch (_) {} state.chart = null; }
  state.chart = Highcharts.chart('chart', buildChartOptions(series, rangeLabel, since, until));
  state.chartRangeName = rangeName || null;
}

// ── Range resolution ──────────────────────────────────────────
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
      return { since: Math.floor(new Date(y, m, d - dow) / 1000), until, label: 'This Week' };
    }
    case 'this_month':
      return { since: Math.floor(new Date(y, m, 1) / 1000), until, label: 'This Month' };
    case 'this_year':
      return { since: Math.floor(new Date(y, 0, 1) / 1000), until, label: 'This Year' };
    case 'past_24h':
      return { since: until - 86400, until, label: 'Past 24 Hours' };
    case 'past_7d':
      return { since: until - 7 * 86400, until, label: 'Past 7 Days' };
    case 'past_30d':
      return { since: until - 30 * 86400, until, label: 'Past 30 Days' };
    case 'past_year':
      return { since: Math.floor(new Date(y - 1, m, d) / 1000), until, label: 'Past Year' };
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

// ── Overview data loading ─────────────────────────────────────
export async function loadRange(name, customSince, customUntil) {
  const { since, until, label } = resolveRange(name, customSince, customUntil);
  try {
    const url = state.apiBase + '/api/data?since=' + since + '&until=' + until;
    const resp = await fetch(url);
    if (!resp.ok) throw new Error('HTTP ' + resp.status);
    const data = await resp.json();

    updateCurrent(data.current);
    updateSummary(data.summary, label);
    const chartSince = data.earliest_at ? Math.max(since, Math.floor(new Date(data.earliest_at) / 1000)) : since;
    renderChart(data.series, label, chartSince, until, name);
  } catch (e) {
    document.getElementById('status').textContent = 'Error: ' + e.message;
  }
}

export async function refreshCurrent() {
  try {
    const resp = await fetch(state.apiBase + '/api/current');
    if (!resp.ok) return;
    updateCurrent(await resp.json());
  } catch (_) {}
}

// ── Range select UI ───────────────────────────────────────────
function todayStr() { return new Date().toISOString().slice(0, 10); }

export function initOverview() {
  const rangeSelect   = document.getElementById('range-select');
  const customRow     = document.getElementById('custom-range');
  const customSinceEl = document.getElementById('custom-since');
  const customUntilEl = document.getElementById('custom-until');
  const applyBtn      = document.getElementById('apply-custom');

  rangeSelect.addEventListener('change', () => {
    const val = rangeSelect.value;
    if (val === 'live') {
      state.isLive = true;
      customRow.classList.remove('visible');
      loadRange('today');
    } else if (val === 'custom') {
      state.isLive = false;
      if (!customSinceEl.value) customSinceEl.value = todayStr();
      if (!customUntilEl.value) customUntilEl.value = todayStr();
      customRow.classList.add('visible');
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
}
