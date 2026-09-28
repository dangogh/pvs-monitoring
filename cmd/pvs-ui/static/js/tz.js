'use strict';

// Timezone-aware calendar arithmetic for bucketing charts into "days", "weeks",
// etc. in the PVS6 site's timezone rather than the viewing browser's. Uses only
// Intl.DateTimeFormat (no moment-timezone or other library) since every
// evergreen browser already ships an IANA tzdata-backed Intl implementation.
//
// `timeZone` is an IANA zone name (e.g. "America/Los_Angeles") or undefined,
// in which case every function here falls back to the browser's own local
// timezone — the pre-existing behavior — so callers don't need to special-case
// a site whose timezone hasn't been configured.

const DOW = { Sun: 0, Mon: 1, Tue: 2, Wed: 3, Thu: 4, Fri: 5, Sat: 6 };

// The wall-clock date/time `date` reads as when displayed in `timeZone`.
export function zonedParts(date, timeZone) {
  const dtf = new Intl.DateTimeFormat('en-US', {
    timeZone,
    hour12: false,
    year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', second: '2-digit',
    weekday: 'short',
  });
  const parts = Object.fromEntries(dtf.formatToParts(date).map(p => [p.type, p.value]));
  return {
    y: +parts.year,
    m: +parts.month - 1,
    d: +parts.day,
    // Midnight renders as hour "24" under hour12:false in some engines.
    h: parts.hour === '24' ? 0 : +parts.hour,
    min: +parts.minute,
    s: +parts.second,
    dow: DOW[parts.weekday],
  };
}

// timeZone's UTC offset (ms, local-minus-UTC) at the instant `utcDate`.
function offsetMs(timeZone, utcDate) {
  const p = zonedParts(utcDate, timeZone);
  const asUTC = Date.UTC(p.y, p.m, p.d, p.h, p.min, p.s);
  return asUTC - utcDate.getTime();
}

// The UTC epoch (ms) for the given wall-clock date/time as read in timeZone —
// the inverse of zonedParts. Two passes: the first guess's own offset can be
// wrong right around a DST transition, so it's corrected once against itself.
export function zonedTimeToUtcMs(y, m, d, h, min, s, timeZone) {
  const guess = Date.UTC(y, m, d, h, min, s);
  const off1 = offsetMs(timeZone, new Date(guess));
  const off2 = offsetMs(timeZone, new Date(guess - off1));
  return guess - off2;
}
