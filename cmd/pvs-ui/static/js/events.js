'use strict';

import { state } from './state.js';

const EVENT_TYPES = [
  { value: 'panel_cleaning', label: 'Panel Cleaning' },
  { value: 'hvac_outage',    label: 'HVAC Outage'    },
  { value: 'other',          label: 'Other'           },
];

function fmtEventType(type) {
  const found = EVENT_TYPES.find(t => t.value === type);
  return found ? found.label : type.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase());
}

export async function fetchMaintenanceEvents() {
  try {
    const resp = await fetch(state.apiBase + '/api/maintenance-events');
    if (!resp.ok) return;
    state.maintenanceEvents = await resp.json();
  } catch (_) {}
}

function renderEventsTable() {
  const tbody = document.getElementById('events-tbody');
  const empty = document.getElementById('events-empty');
  if (!tbody) return;

  const events = state.maintenanceEvents || [];
  tbody.innerHTML = '';

  if (events.length === 0) {
    if (empty) empty.hidden = false;
    return;
  }
  if (empty) empty.hidden = true;

  for (const e of events) {
    const tr = document.createElement('tr');
    const dateStr = fmtEventRange(e.start_at, e.end_at);
    tr.innerHTML =
      '<td>' + escHtml(dateStr) + '</td>' +
      '<td>' + escHtml(fmtEventType(e.event_type)) + '</td>' +
      '<td>' + escHtml(e.notes || '—') + '</td>';
    tbody.appendChild(tr);
  }
}

function fmtDateTime(d) {
  return d.toLocaleDateString([], { month: 'short', day: 'numeric', year: 'numeric' }) +
    ' ' + d.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' });
}

function fmtEventRange(startAt, endAt) {
  const start = new Date(startAt);
  if (!endAt) return fmtDateTime(start);
  const end = new Date(endAt);
  return fmtDateTime(start) + ' – ' + fmtDateTime(end);
}

export function loadEvents() {
  renderEventsTable();
}

function escHtml(s) {
  return String(s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

// Format a Date for an <input type="datetime-local"> value, in local time.
function toDateTimeInputValue(d) {
  const y   = d.getFullYear();
  const mo  = String(d.getMonth() + 1).padStart(2, '0');
  const day = String(d.getDate()).padStart(2, '0');
  const h   = String(d.getHours()).padStart(2, '0');
  const mi  = String(d.getMinutes()).padStart(2, '0');
  return y + '-' + mo + '-' + day + 'T' + h + ':' + mi;
}

// Prefill the event form's start/end from a chart selection (ms since epoch).
export function prefillEventRange(minMs, maxMs) {
  const startEl = document.getElementById('event-start-at');
  const endEl   = document.getElementById('event-end-at');
  const typeSel = document.getElementById('event-type-select');
  if (!startEl || !endEl) return;

  startEl.value = toDateTimeInputValue(new Date(minMs));
  endEl.value   = toDateTimeInputValue(new Date(maxMs));
  typeSel?.focus();
}

export function initEvents() {
  const form       = document.getElementById('event-form');
  const statusEl   = document.getElementById('event-form-status');
  const typeSelect = document.getElementById('event-type-select');

  if (!form || !typeSelect) return;

  EVENT_TYPES.forEach(t => {
    const opt = document.createElement('option');
    opt.value = t.value;
    opt.textContent = t.label;
    typeSelect.appendChild(opt);
  });

  form.addEventListener('submit', async e => {
    e.preventDefault();
    statusEl.textContent = '';
    const startAt   = document.getElementById('event-start-at').value;
    const endAt     = document.getElementById('event-end-at').value;
    const eventType = typeSelect.value;
    const notes     = document.getElementById('event-notes').value.trim();

    if (!startAt || !eventType) return;

    const body = { start_at: new Date(startAt).toISOString(), event_type: eventType };
    if (endAt)  body.end_at = new Date(endAt).toISOString();
    if (notes)  body.notes  = notes;

    try {
      const resp = await fetch(state.apiBase + '/api/maintenance-events', {
        method:  'POST',
        headers: { 'Content-Type': 'application/json' },
        body:    JSON.stringify(body),
      });
      if (!resp.ok) throw new Error('HTTP ' + resp.status);
      await fetchMaintenanceEvents();
      renderEventsTable();
      form.reset();
      statusEl.textContent = 'Event recorded.';
      statusEl.className = 'event-form-status ok';
    } catch (err) {
      statusEl.textContent = 'Error: ' + err.message;
      statusEl.className = 'event-form-status err';
    }
  });
}
