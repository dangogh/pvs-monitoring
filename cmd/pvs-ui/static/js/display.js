'use strict';

// ── Formatting ────────────────────────────────────────────────
export function fmt1(n)  { return n == null ? '—' : n.toFixed(1); }
export function fmt2(n)  { return n == null ? '—' : n.toFixed(2); }
export function fmtKWh(n){ return n == null ? '—' : n.toFixed(2); }

// ── Value display ─────────────────────────────────────────────
// Update a value in place. No fade or flash: the current-power figures change
// on nearly every poll, so animating each refresh just made the dashboard
// strobe. Only touch the DOM when the text actually changed.
export function setValue(el, text) {
  if (el.textContent !== text) el.textContent = text;
}

// ── Clock ─────────────────────────────────────────────────────
export function tickClock() {
  const now = new Date();
  document.getElementById('clock').textContent =
    now.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
}

export function initClock() {
  setInterval(tickClock, 1000);
  tickClock();
}
