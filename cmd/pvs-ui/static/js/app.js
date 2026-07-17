'use strict';

import { state, DEVICES_REFRESH_MS } from './state.js';
import { initClock } from './display.js';
import { loadRange, refreshCurrent, initOverview } from './overview.js';
import { loadPanels, fetchDevices, initPanels } from './panels.js';
import { initMap, loadMap } from './map.js';
import { fetchMaintenanceEvents, initEvents, loadEvents } from './events.js';

// ── Tabs ──────────────────────────────────────────────────────
function switchTab(id) {
  state.activeTab = id;
  document.querySelectorAll('.tab-btn').forEach(b => {
    b.setAttribute('aria-selected', b.getAttribute('aria-controls') === id);
  });
  document.querySelectorAll('.tab-panel').forEach(p => p.classList.toggle('active', p.id === id));
  if (id === 'tab-panels') loadPanels();
  if (id === 'tab-map')    loadMap();
  if (id === 'tab-events') loadEvents();
}

document.querySelectorAll('.tab-btn').forEach(btn =>
  btn.addEventListener('click', () => switchTab(btn.getAttribute('aria-controls')))
);

// ── Init ──────────────────────────────────────────────────────
(async () => {
  try {
    const cfg = await fetch('/config.json').then(r => r.json());
    state.apiBase = (cfg.api_base || '').replace(/\/$/, '');
  } catch (_) {}

  initClock();
  initOverview();
  initPanels();
  initEvents();

  await fetchMaintenanceEvents();
  loadRange('today');
  initMap();
  fetchDevices().catch(() => {});

  setInterval(refreshCurrent, 5000);
  setInterval(() => {
    if (state.activeTab === 'tab-overview') {
      if (state.isLive) loadRange('today');
    } else if (state.activeTab === 'tab-panels') {
      state.panelsFetchedAt = 0;
      loadPanels();
    } else if (state.activeTab === 'tab-map') {
      state.panelsFetchedAt = 0;
      loadMap();
    }
  }, DEVICES_REFRESH_MS);
})();
