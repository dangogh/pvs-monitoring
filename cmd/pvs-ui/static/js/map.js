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
    css = css.replace(/\bbody\b/g, '#map-container');
    const container = document.getElementById('map-container');
    container.innerHTML = '';
    const styleEl = document.createElement('style');
    styleEl.textContent = css;
    container.appendChild(styleEl);
    container.insertAdjacentHTML('beforeend', mapDoc.body.innerHTML);
    document.getElementById('btn-map').style.display = '';
    state.mapLoaded = true;
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

  document.querySelectorAll('#map-container .panel').forEach(el => {
    const label  = el.textContent.trim();
    const pos    = normalisePosition(label);
    const serial = state.positionToSerial[pos];
    const dev    = serial ? devices[serial] : null;

    el.classList.remove('state-working', 'state-error', 'state-other', 'state-unknown');
    if (!dev) {
      el.classList.add('state-unknown');
      el.title = label + ': no data';
    } else {
      el.classList.add(dev.state === 'working' ? 'state-working'
                     : dev.state === 'error'   ? 'state-error'
                     : 'state-other');
      el.title = label + ' · ' + dev.state_descr + ' · ' + fmt1(dev.power_kw) + ' kW';
    }

    el.onclick = () => showMapDetail(el, serial, dev, label);
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
