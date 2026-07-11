'use strict';

import { fmt1 } from './display.js';
import { state, PANELS_TTL_MS } from './state.js';

export async function fetchDevices() {
  const resp = await fetch(state.apiBase + '/api/devices');
  if (!resp.ok) throw new Error('HTTP ' + resp.status);
  state.panelsData = await resp.json();
  state.panelsFetchedAt = Date.now();
}

export async function loadPanels() {
  const stale = state.panelsData.length === 0 || Date.now() - state.panelsFetchedAt > PANELS_TTL_MS;
  if (state.panelsData.length > 0) renderPanels();
  if (!stale) return;

  const container = document.getElementById('panels-container');
  let overlay;
  if (state.panelsData.length === 0) {
    overlay = document.createElement('div');
    overlay.className = 'loading-overlay';
    overlay.innerHTML = '<div class="spinner"></div>';
    container.appendChild(overlay);
  }
  try {
    await fetchDevices();
    renderPanels();
  } catch (e) {
    if (state.panelsData.length === 0)
      document.getElementById('panels-tbody').innerHTML =
        '<tr><td colspan="8" style="text-align:center;color:var(--muted)">Error: ' + e.message + '</td></tr>';
  } finally {
    overlay?.remove();
  }
}

export function renderPanels() {
  const cols = {
    label:        d => state.serialToLabel[d.serial] || '',
    serial:       d => d.serial,
    state:        d => d.state_descr,
    power_kw:     d => d.power_kw,
    today_kwh:    d => d.today_kwh,
    lifetime_kwh: d => d.lifetime_kwh,
    voltage_v:    d => d.voltage_v,
    temp_c:       d => d.temp_c,
  };

  const sorted = [...state.panelsData].sort((a, b) => {
    const av = cols[state.sortCol](a);
    const bv = cols[state.sortCol](b);
    const cmp = typeof av === 'string' ? av.localeCompare(bv, undefined, { numeric: true }) : av - bv;
    return state.sortAsc ? cmp : -cmp;
  });

  const tbody = document.getElementById('panels-tbody');
  const rows = [];
  sorted.forEach(d => {
    const stateClass = d.state === 'working' ? 'state-working'
                     : d.state === 'error'   ? 'state-error'
                     : 'state-other';
    const expanded = state.expandedSerials.has(d.serial);
    const label = state.serialToLabel[d.serial] || '—';
    rows.push(`<tr class="panel-row${expanded ? ' expanded' : ''}" data-serial="${d.serial}" tabindex="0" aria-expanded="${expanded}">
      <td>${label}</td>
      <td class="${stateClass}" style="max-width:6rem;overflow:hidden;text-overflow:ellipsis">${d.state_descr}</td>
      <td>${d.serial}</td>
      <td>${fmt1(d.power_kw)}</td>
      <td>${fmt1(d.today_kwh)}</td>
      <td>${fmt1(d.lifetime_kwh)}</td>
      <td>${fmt1(d.voltage_v)}</td>
      <td>${fmt1(d.temp_c)}</td>
    </tr>`);
    if (expanded) rows.push(detailRow(d));
  });
  tbody.innerHTML = rows.join('');

  const n = state.panelsData.length;
  if (n > 0) {
    const sum = (fn) => state.panelsData.reduce((acc, d) => acc + fn(d), 0);
    const avg = (fn) => sum(fn) / n;
    const ftd = (label, val) =>
      `<td><span style="color:var(--muted);font-weight:400;margin-right:0.3em;font-size:0.72rem">${label}</span>${val}</td>`;
    document.getElementById('panels-tfoot').innerHTML = `<tr>
      <td>${n} panels</td>
      <td></td>
      <td></td>
      ${ftd('total', fmt1(sum(d => d.power_kw)))}
      ${ftd('total', fmt1(sum(d => d.today_kwh)))}
      ${ftd('total', fmt1(sum(d => d.lifetime_kwh)))}
      ${ftd('avg', fmt1(avg(d => d.voltage_v)))}
      ${ftd('avg', fmt1(avg(d => d.temp_c)))}
    </tr>`;
  }

  tbody.querySelectorAll('tr.panel-row').forEach(tr => {
    tr.addEventListener('click', () => togglePanel(tr.dataset.serial));
    tr.addEventListener('keydown', e => {
      if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); togglePanel(tr.dataset.serial); }
    });
  });
}

export function detailRow(d) {
  const fields = [
    { label: 'State',         value: d.state_descr,          unit: ''    },
    { label: 'Power',         value: fmt1(d.power_kw),        unit: 'kW'  },
    { label: 'Today',         value: fmt1(d.today_kwh),       unit: 'kWh' },
    { label: 'Current',       value: fmt1(d.current_a),       unit: 'A'   },
    { label: 'Voltage (AC)',  value: fmt1(d.voltage_v),       unit: 'V'   },
    { label: 'Frequency',     value: fmt1(d.freq_hz),         unit: 'Hz'  },
    { label: 'MPPT1 Power',   value: fmt1(d.power_mppt1_kw),  unit: 'kW'  },
    { label: 'MPPT1 Voltage', value: fmt1(d.voltage_mppt1_v), unit: 'V'   },
    { label: 'MPPT1 Current', value: fmt1(d.current_mppt1_a), unit: 'A'   },
    { label: 'Temperature',   value: fmt1(d.temp_c),          unit: '°C'  },
    { label: 'Lifetime',      value: fmt1(d.lifetime_kwh),    unit: 'kWh' },
  ];
  return `<tr class="detail-row"><td colspan="8"><div class="detail-grid">${
    fields.map(f => `<div class="detail-item">
      <span class="detail-label">${f.label}</span>
      <span class="detail-value">${f.value}${f.unit ? `<span class="detail-unit">${f.unit}</span>` : ''}</span>
    </div>`).join('')
  }</div></td></tr>`;
}

export function togglePanel(serial) {
  if (state.expandedSerials.has(serial)) {
    state.expandedSerials.delete(serial);
  } else {
    state.expandedSerials.add(serial);
  }
  renderPanels();
}

export function initPanels() {
  document.querySelectorAll('#panels-table thead th').forEach(th => {
    th.addEventListener('click', () => {
      const col = th.dataset.col;
      if (state.sortCol === col) {
        state.sortAsc = !state.sortAsc;
      } else {
        state.sortCol = col;
        state.sortAsc = true;
      }
      document.querySelectorAll('#panels-table thead th').forEach(h => h.removeAttribute('aria-sort'));
      th.setAttribute('aria-sort', state.sortAsc ? 'ascending' : 'descending');
      renderPanels();
    });
  });
}
