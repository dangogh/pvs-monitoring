'use strict';

import { state } from './state.js';

// The effective configuration is read-only here: it reflects the DB-backed
// settings the daemon loaded at startup. Editing (and the auth story it needs)
// is deferred — see issue #50. Fields are grouped and given friendly labels;
// the server masks the device-list password before it reaches us.
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

export async function loadConfig() {
  const container = document.getElementById('admin-container');
  if (!container) return;
  container.setAttribute('aria-busy', 'true');
  try {
    const resp = await fetch(state.apiBase + '/api/config');
    if (!resp.ok) throw new Error('HTTP ' + resp.status);
    renderConfig(await resp.json());
  } catch (e) {
    container.innerHTML =
      `<p class="admin-error" role="alert">Could not load configuration: ${e.message}</p>`;
  } finally {
    container.removeAttribute('aria-busy');
  }
}

function renderConfig(cfg) {
  const container = document.getElementById('admin-container');
  const sections = GROUPS.map(group => {
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
  container.innerHTML = sections;
}

function escapeHTML(s) {
  return String(s).replace(/[&<>"']/g, c =>
    ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}
