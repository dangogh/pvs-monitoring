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
    container.innerHTML =
      '<p class="admin-error" role="alert">Could not reach pvs-api.</p>';
    return;
  }
  container.innerHTML = renderStatus(current, version, devices, cfg) + renderConfig(cfg);
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

function renderStatus(current, version, devices, cfg) {
  const items = [];

  // Data freshness: compare the last reading's age against the configured
  // stale threshold (both already on hand — no server-side staleness needed).
  if (current && current.updated_at) {
    const ageMs = Date.now() - new Date(current.updated_at).getTime();
    const staleMs = parseGoDuration(cfg && cfg.stale_threshold) ?? 0;
    const stale = staleMs > 0 && ageMs > staleMs;
    items.push(statusItem('Monitor',
      `${stale ? 'Stale' : 'Live'} · ${humanAge(ageMs)}`,
      stale ? 'stale' : 'live'));
  } else {
    items.push(statusItem('Monitor', 'No data', 'unknown'));
  }

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

// parseGoDuration converts a Go duration string ("5s", "1m0s", "1h30m") to
// milliseconds, or null if it can't be parsed.
function parseGoDuration(s) {
  if (!s) return null;
  const units = { h: 3600000, m: 60000, s: 1000, ms: 1, us: 0.001, 'µs': 0.001, ns: 0.000001 };
  let ms = 0, matched = false;
  for (const m of s.matchAll(/([0-9.]+)(ms|us|µs|ns|h|m|s)/g)) {
    const mult = units[m[2]];
    if (mult === undefined) return null;
    ms += parseFloat(m[1]) * mult;
    matched = true;
  }
  return matched ? ms : null;
}

function humanAge(ms) {
  const s = Math.max(0, Math.round(ms / 1000));
  if (s < 60) return `${s}s ago`;
  const m = Math.round(s / 60);
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  return `${h}h ${m % 60}m ago`;
}

function escapeHTML(s) {
  return String(s).replace(/[&<>"']/g, c =>
    ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}
