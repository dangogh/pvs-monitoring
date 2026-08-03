'use strict';

import { state } from './state.js';

// The Admin tab is read-only: a Status section (daemon health, at a glance) plus
// the effective configuration the daemon loaded at startup. Editing — and the
// auth story it needs — is deferred; see issue #50. All values here come from
// endpoints the rest of the UI already uses, so this adds no new backend.
const GROUPS = [
  {
    title: 'Connection',
    fields: [
      { key: 'addr',                       label: 'PVS6 address' },
      { key: 'stale_threshold',            label: 'Stale threshold' },
      { key: 'reconnect_initial_interval', label: 'Reconnect initial interval' },
      { key: 'reconnect_max_interval',     label: 'Reconnect max interval' },
    ],
  },
  {
    title: 'Device polling',
    fields: [
      { key: 'device_list.url',             label: 'URL' },
      { key: 'device_list.auth_url',        label: 'Auth URL' },
      { key: 'device_list.interval',        label: 'Poll interval' },
      { key: 'device_list.username',        label: 'Username' },
      { key: 'device_list.password',        label: 'Password' },
      { key: 'device_list.tls_fingerprint', label: 'TLS fingerprint' },
    ],
  },
];

export async function loadAdmin() {
  const container = document.getElementById('admin-container');
  if (!container) return;
  container.setAttribute('aria-busy', 'true');

  // Fetch everything in parallel; a single failure shouldn't blank the panel.
  const base = state.apiBase;
  const [cfg, current, version, devices] = await Promise.all([
    getJSON(base + '/api/config'),
    getJSON(base + '/api/current'),   // 503 "no data" before the first reading
    getJSON(base + '/api/version'),
    getJSON(base + '/api/devices'),
  ]);
  container.removeAttribute('aria-busy');

  if (!cfg && !current && !version && !devices) {
    stopFreshnessTicker();
    container.innerHTML =
      '<p class="admin-error" role="alert">Could not reach pvs-api.</p>';
    return;
  }
  // Seed the shared reading so the Monitor line paints immediately, even before
  // the app-wide 5s poll (refreshCurrent) has run once.
  if (current) state.current = current;
  container.innerHTML = renderStatus(version, devices) + renderConfig(cfg);
  startFreshnessTicker();
}

async function getJSON(url) {
  try {
    const resp = await fetch(url);
    if (!resp.ok) return null;
    return await resp.json();
  } catch (_) {
    return null;
  }
}

function renderStatus(version, devices) {
  const items = [];

  // Monitor freshness gets a stable id so the 1s ticker can refresh just this
  // value in place (see startFreshnessTicker) without re-fetching or re-rendering.
  const mon = monitorStatus(state.current);
  items.push(`<div class="detail-item">
    <span class="detail-label">Monitor</span>
    <span id="admin-monitor" class="detail-value status-value--${mon.tone}">${escapeHTML(mon.text)}</span>
  </div>`);

  if (version && version.version) {
    items.push(statusItem('Version', version.version, ''));
  }

  if (Array.isArray(devices)) {
    const total = devices.length;
    const working = devices.filter(d => d.state === 'working').length;
    const errored = devices.filter(d => d.state === 'error').length;
    const tone = errored > 0 ? 'stale' : total > 0 ? 'live' : 'unknown';
    const extra = errored > 0 ? ` · ${errored} error` : '';
    items.push(statusItem('Panels', `${working}/${total} reporting${extra}`, tone));
  }

  return `<section class="admin-group" aria-label="Status">
    <h2 class="admin-group-title">Status</h2>
    <div class="detail-grid">${items.join('')}</div>
  </section>`;
}

function statusItem(label, value, tone) {
  const cls = tone ? ` status-value--${tone}` : '';
  return `<div class="detail-item">
    <span class="detail-label">${label}</span>
    <span class="detail-value${cls}">${escapeHTML(value)}</span>
  </div>`;
}

// monitorStatus derives the Monitor freshness badge from the latest reading.
// While data is flowing it reads a calm, static "Live" — no ticking seconds,
// which would only sawtooth 0→5s against the reading poll cadence without saying
// anything useful. The age appears only once the feed goes stale (>2 min), where
// "how long since the last reading" is the thing you actually want to know. The
// daemon's stale_threshold (seconds) governs MCP-tool semantics, not this badge.
const STALE_AFTER_S = 120;

function monitorStatus(reading) {
  if (!reading || !reading.updated_at) return { text: 'No data', tone: 'unknown' };
  const age = Math.round((Date.now() - new Date(reading.updated_at).getTime()) / 1000);
  if (age > STALE_AFTER_S) return { text: `Stale · ${ageLabel(age)}`, tone: 'stale' };
  return { text: 'Live', tone: 'live' };
}

// The freshness ticker recomputes the Monitor age once a second from the shared
// reading (refreshed every 5s by the app-wide poll), so "Xs ago" counts honestly
// instead of freezing between the 30s Admin reloads. It self-stops when the Admin
// tab is no longer showing, so at most one interval is ever live.
let freshTimer = null;

function startFreshnessTicker() {
  stopFreshnessTicker();
  freshTimer = setInterval(tickFreshness, 1000);
}

function stopFreshnessTicker() {
  if (freshTimer) { clearInterval(freshTimer); freshTimer = null; }
}

function tickFreshness() {
  const el = document.getElementById('admin-monitor');
  if (!el || state.activeTab !== 'tab-admin') { stopFreshnessTicker(); return; }
  const mon = monitorStatus(state.current);
  el.textContent = mon.text;
  el.className = 'detail-value status-value--' + mon.tone;
}

function renderConfig(cfg) {
  if (!cfg) {
    return '<p class="admin-error" role="alert">Could not load configuration.</p>';
  }
  return GROUPS.map(group => {
    const items = group.fields.map(f => {
      const raw = cfg[f.key];
      const empty = raw === undefined || raw === '';
      const value = empty ? 'not set' : raw;
      return `<div class="detail-item">
        <span class="detail-label">${f.label}</span>
        <span class="detail-value${empty ? ' detail-value--empty' : ''}">${escapeHTML(value)}</span>
      </div>`;
    }).join('');
    return `<section class="admin-group" aria-label="${group.title}">
      <h2 class="admin-group-title">${group.title}</h2>
      <div class="detail-grid">${items}</div>
    </section>`;
  }).join('');
}

// ageLabel formats a stale reading's age (seconds); only reached once the feed
// has been quiet for over STALE_AFTER_S, so minute granularity is the floor.
function ageLabel(s) {
  if (s < 3600)  return Math.floor(s / 60) + 'm ago';
  if (s < 86400) return Math.floor(s / 3600) + 'h ago';
  return Math.floor(s / 86400) + 'd ago';
}

function escapeHTML(s) {
  return String(s).replace(/[&<>"']/g, c =>
    ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}
