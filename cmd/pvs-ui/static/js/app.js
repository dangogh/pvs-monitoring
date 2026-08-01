'use strict';

import { state, DEVICES_REFRESH_MS } from './state.js';
import { initClock } from './display.js';
import { loadRange, refreshCurrent, initOverview, fetchAndRender } from './overview.js';
import { loadPanels, fetchDevices, initPanels } from './panels.js';
import { initMap, loadMap, initMapAnimation, syncMapRange } from './map.js';
import { fetchMaintenanceEvents, initEvents, loadEvents } from './events.js';

// ── Tabs ──────────────────────────────────────────────────────
function switchTab(id, focusTab = false) {
  const prev = state.activeTab;
  state.activeTab = id;
  document.querySelectorAll('.tab-btn').forEach(b => {
    const selected = b.getAttribute('aria-controls') === id;
    b.setAttribute('aria-selected', selected);
    // Roving tabindex: only the selected tab is in the tab order; arrow keys
    // move between the others (ARIA tabs pattern).
    b.tabIndex = selected ? 0 : -1;
    if (selected && focusTab) b.focus();
  });
  document.querySelectorAll('.tab-panel').forEach(p => p.classList.toggle('active', p.id === id));
  if (id === 'tab-panels') loadPanels();
  if (id === 'tab-map')    { syncMapRange(); loadMap(); }
  if (id === 'tab-events') loadEvents();
  if (id === 'tab-overview' && prev === 'tab-map' && state.lastSince) {
    fetchAndRender(state.lastSince, state.lastUntil, null, 'custom');
  }
}

// Only tabs whose button is actually shown are navigable (the Map tab is hidden
// until a site map is configured).
function visibleTabs() {
  return [...document.querySelectorAll('.tab-btn')].filter(b => b.offsetParent !== null);
}

document.querySelectorAll('.tab-btn').forEach(btn => {
  btn.addEventListener('click', () => switchTab(btn.getAttribute('aria-controls')));
  btn.addEventListener('keydown', e => {
    const tabs = visibleTabs();
    const i = tabs.indexOf(btn);
    if (i === -1) return;
    let j = null;
    if (e.key === 'ArrowRight' || e.key === 'ArrowDown') j = (i + 1) % tabs.length;
    else if (e.key === 'ArrowLeft' || e.key === 'ArrowUp') j = (i - 1 + tabs.length) % tabs.length;
    else if (e.key === 'Home') j = 0;
    else if (e.key === 'End') j = tabs.length - 1;
    if (j === null) return;
    e.preventDefault();
    switchTab(tabs[j].getAttribute('aria-controls'), true);
  });
});

// Fetch the app version from pvs-api and show it in the footer.
async function loadVersion() {
  const el = document.getElementById('app-version');
  if (!el) return;
  try {
    const resp = await fetch(state.apiBase + '/api/version');
    if (!resp.ok) return;
    const { version } = await resp.json();
    if (version) el.textContent = 'pvs-monitoring ' + version;
  } catch (_) {}
}

// ── Init ──────────────────────────────────────────────────────
(async () => {
  try {
    const cfg = await fetch('/config.json').then(r => r.json());
    state.apiBase = (cfg.api_base || '').replace(/\/$/, '');
  } catch (_) {}

  loadVersion();
  initClock();
  initOverview();
  initPanels();
  initEvents();

  await fetchMaintenanceEvents();
  loadRange('today');
  initMap();
  initMapAnimation();
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
