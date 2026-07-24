'use strict';

import { fmt1 } from './display.js';
import { state, PANELS_TTL_MS } from './state.js';
import { fetchDevices } from './panels.js';

export function normalisePosition(pos) {
  // 'C02' → 'C2', 'B20' → 'B20'
  return pos.replace(/^([A-Za-z]+)0*(\d+)$/, (_, l, n) => l.toUpperCase() + parseInt(n, 10));
}

export function parseCsv(csvText) {
  const result = { positionToSerial: {}, serialToLabel: {} };
  csvText.split('\n').slice(1).forEach(line => {
    const parts = line.split(',');
    if (parts.length >= 2) {
      const pos    = normalisePosition(parts[0].trim());
      const serial = parts[1].trim();
      if (pos && serial) {
        result.positionToSerial[pos] = serial;
        result.serialToLabel[serial] = pos;
      }
    }
  });
  return result;
}

export async function initMap() {
  try {
    const [mapResp, csvResp] = await Promise.all([
      fetch(state.apiBase + '/assets/map.html'),
      fetch(state.apiBase + '/assets/map.csv'),
    ]);
    if (!mapResp.ok || !csvResp.ok) return;

    const mapHtml = await mapResp.text();
    const csvText = await csvResp.text();

    const { positionToSerial, serialToLabel } = parseCsv(csvText);
    Object.assign(state.positionToSerial, positionToSerial);
    Object.assign(state.serialToLabel, serialToLabel);

    const mapDoc = new DOMParser().parseFromString(mapHtml, 'text/html');
    let css = '';
    mapDoc.querySelectorAll('style').forEach(s => css += s.textContent);
    // Strip background/color from the body rule so the app's own dark-theme
    // styling for #map-container (index.html) isn't clobbered by the
    // site-specific map.html's hardcoded white background.
    css = css.replace(/body\s*\{([^}]*)\}/, (_, decls) => {
      const cleaned = decls.replace(/\s*background\s*:[^;]+;?/gi, '').replace(/\s*color\s*:[^;]+;?/gi, '');
      return `#map-container {${cleaned}}`;
    });
    css = css.replace(/\bbody\b/g, '#map-container');
    const container = document.getElementById('map-container');
    container.innerHTML = '';
    const styleEl = document.createElement('style');
    styleEl.textContent = css;
    container.appendChild(styleEl);
    container.insertAdjacentHTML('beforeend', mapDoc.body.innerHTML);
    document.getElementById('btn-map').style.display = '';
    state.mapLoaded = true;
    state.mapPanelEls = null;
  } catch (_) {}
}

export async function loadMap() {
  if (!state.mapLoaded) return;
  const stale = state.panelsData.length === 0 || Date.now() - state.panelsFetchedAt > PANELS_TTL_MS;
  let overlay;
  if (state.panelsData.length === 0) {
    overlay = document.createElement('div');
    overlay.className = 'loading-overlay';
    overlay.innerHTML = '<div class="spinner"></div>';
    document.getElementById('map-layout').appendChild(overlay);
  }
  try {
    if (stale) await fetchDevices();
  } catch (_) {}
  finally { overlay?.remove(); }
  const devices = Object.fromEntries(state.panelsData.map(d => [d.serial, d]));

  if (!state.mapPanelEls) {
    state.mapPanelEls = Array.from(document.querySelectorAll('#map-container .panel')).map(el => ({
      el, label: el.textContent.trim(),
    }));
  }

  state.mapPanelEls.forEach(({ el, label }) => {
    const pos    = normalisePosition(label);
    const serial = state.positionToSerial[pos];
    const dev    = serial ? devices[serial] : null;

    const stateClass = !dev ? 'state-unknown'
                     : dev.state === 'working' ? 'state-working'
                     : dev.state === 'error'   ? 'state-error'
                     : 'state-other';
    const title = !dev ? label + ': no data'
                        : label + ' · ' + dev.state_descr + ' · ' + fmt1(dev.power_kw) + ' kW';

    if (!el.classList.contains(stateClass)) {
      el.classList.remove('state-working', 'state-error', 'state-other', 'state-unknown');
      el.classList.add(stateClass);
    }
    if (el.title !== title) el.title = title;

    el._panelSerial = serial;
    el._panelDev = dev;
    if (!el.onclick) {
      el.onclick = () => showMapDetail(el, el._panelSerial, el._panelDev, label);
    }
  });
}

function detailField(label, value, unit) {
  return `<div class="detail-item">
    <span class="detail-label">${label}</span>
    <span class="detail-value">${value}${unit ? `<span class="detail-unit">${unit}</span>` : ''}</span>
  </div>`;
}

function detailSection(label, fields) {
  return `<div class="map-detail-section">
    <div class="map-detail-section-label">${label}</div>
    <div class="detail-grid">${fields.map(f => detailField(f.label, f.value, f.unit)).join('')}</div>
  </div>`;
}

// ── Animation ─────────────────────────────────────────────────────────────────

const anim = {
  frames: [],      // [{timeMS, bySerial: {serial → powerKW}}]
  frameIdx: 0,
  playing: false,
  timer: null,
};

function localISONoSec(d) {
  const pad = n => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth()+1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

function todayMidnight() {
  const d = new Date();
  d.setHours(0, 0, 0, 0);
  return d;
}

// Map t ∈ [0,1] to a color: gray(0) → amber(0.5) → green(1)
function powerColor(t) {
  if (t <= 0.5) {
    const s = t * 2;
    const r = Math.round(107 + (251 - 107) * s);
    const g = Math.round(114 + (191 - 114) * s);
    const b = Math.round(128 + (36  - 128) * s);
    return [`rgb(${r},${g},${b})`, s < 0.4 ? '#d1d5db' : '#1c1917'];
  } else {
    const s = (t - 0.5) * 2;
    const r = Math.round(251 + (22  - 251) * s);
    const g = Math.round(191 + (163 - 191) * s);
    const b = Math.round(36  + (74  - 36)  * s);
    return [`rgb(${r},${g},${b})`, '#052e16'];
  }
}

function renderFrame(idx) {
  const frame = anim.frames[idx];
  if (!frame || !state.mapPanelEls) return;

  const values = Object.values(frame.bySerial);
  const max = values.length ? Math.max(...values) : 0;

  state.mapPanelEls.forEach(({ el, label }) => {
    const pos    = normalisePosition(label);
    const serial = state.positionToSerial[pos];
    const power  = serial != null ? (frame.bySerial[serial] ?? null) : null;

    if (power === null || max === 0) {
      el.style.removeProperty('background');
      el.style.removeProperty('color');
      el.classList.remove('state-working', 'state-error', 'state-other');
      el.classList.add('state-unknown');
    } else {
      const t = max > 0 ? power / max : 0;
      const [bg, fg] = powerColor(t);
      el.style.setProperty('background', bg, 'important');
      el.style.setProperty('color', fg, 'important');
      el.classList.remove('state-working', 'state-error', 'state-other', 'state-unknown');
    }
    el.title = serial && power !== null
      ? `${label} · ${fmt1(power)} kW (${max > 0 ? Math.round(power / max * 100) : 0}%)`
      : `${label}: no data`;
  });

  const scrubber = document.getElementById('anim-scrubber');
  if (scrubber) scrubber.value = idx;

  const ts = document.getElementById('anim-timestamp');
  if (ts) ts.textContent = new Date(frame.timeMS).toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
}

function animStep(dir = 1) {
  anim.frameIdx = Math.max(0, Math.min(anim.frames.length - 1, anim.frameIdx + dir));
  renderFrame(anim.frameIdx);
  if (anim.playing && anim.frameIdx >= anim.frames.length - 1) animPause();
}

function animPlay() {
  if (!anim.frames.length) return;
  anim.playing = true;
  document.getElementById('anim-btn-play').textContent = '⏸';
  anim.timer = setInterval(() => animStep(1), 300);
}

function animPause() {
  anim.playing = false;
  clearInterval(anim.timer);
  anim.timer = null;
  const btn = document.getElementById('anim-btn-play');
  if (btn) btn.innerHTML = '&#9654;';
}

function animToggle() {
  if (anim.playing) animPause(); else {
    if (anim.frameIdx >= anim.frames.length - 1) anim.frameIdx = 0;
    animPlay();
  }
}

function buildFrames(pts, sinceMS, untilMS) {
  const BUCKET_MS = 5 * 60 * 1000;
  const byTime = new Map();
  for (const p of pts) {
    let frame = byTime.get(p.t);
    if (!frame) { frame = { timeMS: p.t, bySerial: {} }; byTime.set(p.t, frame); }
    frame.bySerial[p.serial] = p.p;
  }
  // Fill all 5-min buckets in the requested range so timestamp always matches picker
  const start = Math.ceil(sinceMS / BUCKET_MS) * BUCKET_MS;
  for (let t = start; t <= untilMS; t += BUCKET_MS) {
    if (!byTime.has(t)) byTime.set(t, { timeMS: t, bySerial: {} });
  }
  return Array.from(byTime.values()).sort((a, b) => a.timeMS - b.timeMS);
}

async function loadAnimSeries() {
  animPause();
  const since = document.getElementById('anim-since').value;
  const until = document.getElementById('anim-until').value;
  if (!since || !until) return;
  const s = Math.floor(new Date(since).getTime() / 1000);
  const u = Math.floor(new Date(until).getTime() / 1000);
  if (s >= u) return;
  state.lastSince = s;
  state.lastUntil = u;

  try {
    const resp = await fetch(`${state.apiBase}/api/inverter-series?since=${s}&until=${u}`);
    if (!resp.ok) return;
    const pts = await resp.json();
    anim.frames = buildFrames(pts ?? [], s * 1000, u * 1000);
    anim.frameIdx = 0;
    const scrubber = document.getElementById('anim-scrubber');
    if (scrubber) scrubber.max = Math.max(0, anim.frames.length - 1);
    document.getElementById('map-container').classList.add('anim-mode');
    if (anim.frames.length) renderFrame(0);
  } catch (_) {}
}

function shiftWindow(factor) {
  const sinceEl = document.getElementById('anim-since');
  const untilEl = document.getElementById('anim-until');
  if (!sinceEl.value || !untilEl.value) return;
  const s = new Date(sinceEl.value).getTime();
  const u = new Date(untilEl.value).getTime();
  const span = u - s;
  const newSpan = span * factor;
  const mid = (s + u) / 2;
  sinceEl.value = localISONoSec(new Date(mid - newSpan / 2));
  untilEl.value = localISONoSec(new Date(mid + newSpan / 2));
}

export function syncMapRange() {
  const sinceEl = document.getElementById('anim-since');
  const untilEl = document.getElementById('anim-until');
  if (state.lastSince) {
    sinceEl.value = localISONoSec(new Date(state.lastSince * 1000));
    untilEl.value = localISONoSec(new Date(state.lastUntil * 1000));
  } else {
    sinceEl.value = localISONoSec(todayMidnight());
    untilEl.value = localISONoSec(new Date());
  }
}

export function initMapAnimation() {
  syncMapRange();

  document.getElementById('anim-btn-play').addEventListener('click', animToggle);
  document.getElementById('anim-btn-step-back').addEventListener('click', () => { animPause(); animStep(-1); });
  document.getElementById('anim-btn-step-fwd').addEventListener('click', () => { animPause(); animStep(1); });
  document.getElementById('anim-load').addEventListener('click', loadAnimSeries);
  document.getElementById('anim-half').addEventListener('click', () => shiftWindow(0.5));
  document.getElementById('anim-double').addEventListener('click', () => shiftWindow(2));
  document.getElementById('anim-scrubber').addEventListener('input', e => {
    animPause();
    anim.frameIdx = parseInt(e.target.value, 10);
    renderFrame(anim.frameIdx);
  });
}

export function showMapDetail(el, serial, dev, label) {
  document.querySelectorAll('#map-container .panel.selected').forEach(p => p.classList.remove('selected'));
  el.classList.add('selected');

  document.getElementById('map-detail-title').textContent = label + (serial ? ' · ' + serial.slice(-6) : '');

  if (!dev) {
    document.getElementById('map-detail-grid').innerHTML =
      '<div class="detail-item"><span class="detail-label">Status</span><span class="detail-value">No data</span></div>';
  } else {
    document.getElementById('map-detail-grid').innerHTML = [
      detailSection('AC Output', [
        { label: 'Power',    value: fmt1(dev.power_kw),   unit: 'kW'  },
        { label: 'Today',    value: fmt1(dev.today_kwh),  unit: 'kWh' },
        { label: 'Current',  value: fmt1(dev.current_a),  unit: 'A'   },
        { label: 'Voltage',  value: fmt1(dev.voltage_v),  unit: 'V'   },
        { label: 'Freq',     value: fmt1(dev.freq_hz),    unit: 'Hz'  },
      ]),
      detailSection('DC Input', [
        { label: 'Power',    value: fmt1(dev.power_mppt1_kw),   unit: 'kW' },
        { label: 'Voltage',  value: fmt1(dev.voltage_mppt1_v),  unit: 'V'  },
        { label: 'Current',  value: fmt1(dev.current_mppt1_a),  unit: 'A'  },
      ]),
      detailSection('Info', [
        { label: 'State',    value: dev.state_descr,        unit: ''    },
        { label: 'Temp',     value: fmt1(dev.temp_c),        unit: '°C'  },
        { label: 'Lifetime', value: fmt1(dev.lifetime_kwh),  unit: 'kWh' },
      ]),
    ].join('');
  }

  document.getElementById('map-detail').classList.add('visible');
}
