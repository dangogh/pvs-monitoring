'use strict';

import { state } from './state.js';

const EVENT_TYPES = [
  { value: 'panel_cleaning', label: 'Panel Cleaning' },
  { value: 'soiling',        label: 'Soiling'        },
  { value: 'weather',        label: 'Weather'        },
  { value: 'hvac_outage',    label: 'HVAC Outage'    },
  { value: 'inverter_outage', label: 'Inverter Outage' },
  { value: 'grid_outage',    label: 'Grid Outage'    },
  { value: 'maintenance',    label: 'Maintenance'    },
  // Load-side: an expected rise in consumption, not an array fault.
  { value: 'ev_charging',    label: 'EV Charging'    },
  { value: 'other',          label: 'Other'          },
];

// ID of the event currently being edited, or null when creating a new one.
let editingId = null;

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

    const actions = document.createElement('td');

    const editBtn = document.createElement('button');
    editBtn.type = 'button';
    editBtn.className = 'event-edit';
    editBtn.textContent = 'Edit';
    editBtn.setAttribute('aria-label', 'Edit event: ' + fmtEventType(e.event_type) + ' on ' + dateStr);
    editBtn.addEventListener('click', () => startEdit(e));
    actions.appendChild(editBtn);

    const delBtn = document.createElement('button');
    delBtn.type = 'button';
    delBtn.className = 'event-delete';
    delBtn.textContent = 'Delete';
    delBtn.setAttribute('aria-label', 'Delete event: ' + fmtEventType(e.event_type) + ' on ' + dateStr);
    delBtn.addEventListener('click', () => deleteEvent(e.id, delBtn));
    actions.appendChild(delBtn);

    tr.appendChild(actions);
    tbody.appendChild(tr);
  }
}

// Enter edit mode: prefill the form from an existing event.
function startEdit(e) {
  editingId = e.id;
  document.getElementById('event-start-at').value = toDateTimeInputValue(new Date(e.start_at));
  document.getElementById('event-end-at').value   = e.end_at ? toDateTimeInputValue(new Date(e.end_at)) : '';
  document.getElementById('event-type-select').value = e.event_type;
  document.getElementById('event-notes').value    = e.notes || '';

  document.getElementById('event-form-heading').textContent = 'Edit Event';
  document.getElementById('event-submit').textContent = 'Save Changes';
  document.getElementById('event-cancel').hidden = false;
  document.getElementById('event-form').scrollIntoView({ behavior: 'smooth', block: 'nearest' });
  document.getElementById('event-start-at').focus();
}

// Leave edit mode and return the form to "create new" state.
function exitEdit() {
  editingId = null;
  document.getElementById('event-form').reset();
  document.getElementById('event-form-heading').textContent = 'Record New Event';
  document.getElementById('event-submit').textContent = 'Save Event';
  document.getElementById('event-cancel').hidden = true;
}

async function deleteEvent(id, btn) {
  if (!confirm('Delete this event? This cannot be undone.')) return;
  btn.disabled = true;
  try {
    const resp = await fetch(state.apiBase + '/api/maintenance-events/' + id, { method: 'DELETE' });
    if (!resp.ok && resp.status !== 404) throw new Error('HTTP ' + resp.status);
    await fetchMaintenanceEvents();
    renderEventsTable();
  } catch (err) {
    btn.disabled = false;
    alert('Could not delete event: ' + err.message);
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

  document.getElementById('event-cancel')?.addEventListener('click', () => {
    exitEdit();
    statusEl.textContent = '';
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

    const editing = editingId !== null;
    const url = editing
      ? state.apiBase + '/api/maintenance-events/' + editingId
      : state.apiBase + '/api/maintenance-events';

    try {
      const resp = await fetch(url, {
        method:  editing ? 'PATCH' : 'POST',
        headers: { 'Content-Type': 'application/json' },
        body:    JSON.stringify(body),
      });
      if (!resp.ok) throw new Error('HTTP ' + resp.status);
      await fetchMaintenanceEvents();
      renderEventsTable();
      exitEdit();
      statusEl.textContent = editing ? 'Event updated.' : 'Event recorded.';
      statusEl.className = 'event-form-status ok';
    } catch (err) {
      statusEl.textContent = 'Error: ' + err.message;
      statusEl.className = 'event-form-status err';
    }
  });
}
