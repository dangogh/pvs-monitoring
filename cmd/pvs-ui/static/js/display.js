'use strict';

// ── Formatting ────────────────────────────────────────────────
export function fmt1(n)  { return n == null ? '—' : n.toFixed(1); }
export function fmt2(n)  { return n == null ? '—' : n.toFixed(2); }
export function fmtKWh(n){ return n == null ? '—' : n.toFixed(2); }

// ── Animation helpers ─────────────────────────────────────────
export function setValueAnimated(el, text) {
  el.style.opacity = '0.3';
  setTimeout(() => { el.textContent = text; el.style.opacity = ''; }, 150);
}

export function flashCard(el) {
  el.classList.remove('card-flash');
  requestAnimationFrame(() => requestAnimationFrame(() => {
    el.classList.add('card-flash');
    setTimeout(() => el.classList.remove('card-flash'), 50);
  }));
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
