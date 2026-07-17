'use strict';

// Shared mutable state — imported by all other modules.
// No imports here; this module must remain dependency-free.
export const state = {
  apiBase:         '',
  chart:           null,
  chartRangeName:  null,
  isLive:          true,
  currentRange:    'today',
  activeTab:       'tab-overview',
  lastSince:       null,
  lastUntil:       null,

  // Panels
  panelsData:      [],
  panelsFetchedAt: 0,
  sortCol:         'label',
  sortAsc:         true,
  expandedSerials: new Set(),

  // Map
  positionToSerial: {},
  serialToLabel:    {},
  mapLoaded:        false,
};

export const DEVICES_REFRESH_MS = 30_000;
export const PANELS_TTL_MS      = DEVICES_REFRESH_MS;
