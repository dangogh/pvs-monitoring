'use strict';

// ── Formatting ────────────────────────────────────────────────
function fmt1(n)  { return n == null ? '—' : n.toFixed(1); }
function fmt2(n)  { return n == null ? '—' : n.toFixed(2); }
function fmtKWh(n){ return n == null ? '—' : n.toFixed(2); }

// ── Animation helpers ─────────────────────────────────────────
function setValueAnimated(el, text) {
  el.style.opacity = '0.3';
  setTimeout(() => { el.textContent = text; el.style.opacity = ''; }, 150);
}

function flashCard(el) {
  el.classList.remove('card-flash');
  requestAnimationFrame(() => requestAnimationFrame(() => {
    el.classList.add('card-flash');
    setTimeout(() => el.classList.remove('card-flash'), 50);
  }));
}

// ── Clock ─────────────────────────────────────────────────────
function tickClock() {
  const now = new Date();
  document.getElementById('clock').textContent =
    now.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
}
setInterval(tickClock, 1000);
tickClock();
