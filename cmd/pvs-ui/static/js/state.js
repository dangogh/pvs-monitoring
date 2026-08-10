'use strict';

// Shared mutable state — imported by all other modules.
// No imports here; this module must remain dependency-free.
export const state = {
  apiBase:         '',
  chart:           null,
  chartRangeName:  null,
  isLive:          true,
  current:         null,   // latest /api/current reading (refreshed every 5s)
  currentRange:    'today',
  activeTab:       'tab-overview',
  lastSince:       null,
  lastUntil:       null,

  // Chart smoothing: independent moving-average windows (seconds; 0 = off/raw)
  // for the production and usage lines. The available windows scale with the
  // visible span (see smoothingOptionsFor); values snap to the nearest option
  // when the range changes. Usage defaults smoothed, production raw.
  smoothingSolar:  0,
  smoothingLoad:   600,

  // Panels
  panelsData:      [],
  panelsFetchedAt: 0,
  panelsPaused:    false,
  sortCol:         'label',
  sortAsc:         true,
  expandedSerials: new Set(),

  // Map
  positionToSerial: {},
  serialToLabel:    {},
  mapLoaded:        false,

  // Maintenance events
  maintenanceEvents: [],
};

export const DEVICES_REFRESH_MS = 30_000;
export const PANELS_TTL_MS      = DEVICES_REFRESH_MS;
