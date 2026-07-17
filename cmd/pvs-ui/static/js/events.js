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
    const dateStr = e.end_date && e.end_date !== e.start_date
      ? e.start_date + ' – ' + e.end_date
      : e.start_date;
    tr.innerHTML =
      '<td>' + escHtml(dateStr) + '</td>' +
      '<td>' + escHtml(fmtEventType(e.event_type)) + '</td>' +
      '<td>' + escHtml(e.notes || '—') + '</td>';
    tbody.appendChild(tr);
  }
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
    const startDate = document.getElementById('event-start-date').value;
    const endDate   = document.getElementById('event-end-date').value;
    const eventType = typeSelect.value;
    const notes     = document.getElementById('event-notes').value.trim();

    if (!startDate || !eventType) return;

    const body = { start_date: startDate, event_type: eventType };
    if (endDate)  body.end_date = endDate;
    if (notes)    body.notes    = notes;

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
