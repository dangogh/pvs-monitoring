'use strict';

// ── Shared state ──────────────────────────────────────────────
let API_BASE  = '';
let chart     = null;
let isLive    = true;
let currentRange = 'today';
let activeTab = 'tab-overview';

const DEVICES_REFRESH_MS = 30_000;
const PANELS_TTL_MS      = DEVICES_REFRESH_MS;

// ── Tabs ──────────────────────────────────────────────────────
const tabBtns   = document.querySelectorAll('.tab-btn');
const tabPanels = document.querySelectorAll('.tab-panel');

function switchTab(id) {
  activeTab = id;
  tabBtns.forEach(b => {
    const active = b.getAttribute('aria-controls') === id;
    b.setAttribute('aria-selected', active);
  });
  tabPanels.forEach(p => p.classList.toggle('active', p.id === id));
  if (id === 'tab-panels') loadPanels();
  if (id === 'tab-map')    loadMap();
}

tabBtns.forEach(btn => btn.addEventListener('click', () => switchTab(btn.getAttribute('aria-controls'))));

// ── Init ──────────────────────────────────────────────────────
(async () => {
  try {
    const cfg = await fetch('/config.json').then(r => r.json());
    API_BASE = (cfg.api_base || '').replace(/\/$/, '');
  } catch (_) {}
  loadRange('today');
  initMap();
  fetchDevices().catch(() => {});
  setInterval(refreshCurrent, 5000);
  setInterval(() => {
    if (activeTab === 'tab-overview') {
      if (isLive) loadRange('today');
    } else if (activeTab === 'tab-panels') {
      panelsFetchedAt = 0;
      loadPanels();
    } else if (activeTab === 'tab-map') {
      panelsFetchedAt = 0;
      loadMap();
    }
  }, DEVICES_REFRESH_MS);
})();
