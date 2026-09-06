'use strict';

// Peer-relative panel output.
//
// A panel's raw kW says little on its own: it rises and falls with irradiance,
// time of day, and season along with every other panel. Dividing by the median
// of its peer group at the same instant cancels all of that, so what remains is
// the panel's own deviation — which is what a fault looks like.
//
// A peer group is the set of panels that *should* produce the same power at the
// same moment. That is decided by shading, not by circuit: panels sharing a
// roof plane and shadow track each other, while panels on one breaker can be
// shaded at opposite ends of the day. Groups come from the map.csv column
// headed "group" (see parseCsv); panels without one share the UNGROUPED pool,
// which still catches a hard failure even though its median is noisier.

// Fleet median below this (kW) means there is not enough light to tell a fault
// from nightfall — every panel is near zero and the ratio is noise.
export const LOW_LIGHT_KW = 0.10;

// Below this fraction of the peer median a panel is flagged as underperforming.
export const UNDERPERFORM_RATIO = 0.60;

export const UNGROUPED = 'ungrouped';

export function median(values) {
  const v = values.filter(x => typeof x === 'number' && Number.isFinite(x)).sort((a, b) => a - b);
  if (v.length === 0) return NaN;
  const mid = v.length >> 1;
  return v.length % 2 ? v[mid] : (v[mid - 1] + v[mid]) / 2;
}

// Median power per peer group, plus the fleet-wide median used for the
// low-light gate. Groups are keyed by the value in serialToGroup.
export function peerMedians(panels, serialToGroup = {}) {
  const byGroup = {};
  panels.forEach(d => {
    const g = serialToGroup[d.serial] || UNGROUPED;
    (byGroup[g] ||= []).push(d.power_kw);
  });
  const medians = {};
  Object.keys(byGroup).forEach(g => { medians[g] = median(byGroup[g]); });
  return { medians, fleet: median(panels.map(d => d.power_kw)) };
}

// Ratio of one panel to its peer group's median.
//
// Returns { ratio, dark, group }. `dark` is true when the fleet is producing so
// little that no comparison is meaningful; callers must render that state
// explicitly rather than printing a number, since near-zero denominators
// produce wild percentages. `ratio` is NaN whenever it cannot be computed.
export function peerRatio(device, ctx, serialToGroup = {}) {
  const group = serialToGroup[device.serial] || UNGROUPED;
  if (!(ctx.fleet >= LOW_LIGHT_KW)) return { ratio: NaN, dark: true, group };
  const med = ctx.medians[group];
  if (!(med > 0)) return { ratio: NaN, dark: true, group };
  return { ratio: device.power_kw / med, dark: false, group };
}

// Whether a computed ratio counts as underperforming. The single definition:
// the table, the map, and underperformers() all ask this rather than repeating
// the comparison, so the threshold has one place to change.
export function isLow({ ratio, dark }, threshold = UNDERPERFORM_RATIO) {
  return !dark && Number.isFinite(ratio) && ratio < threshold;
}

// Panels producing far less than their peers. Stateless by design: a caller
// that raises an alarm should require the same serials on consecutive polls,
// because a passing cloud can dip one panel for a single reading.
export function underperformers(panels, serialToGroup = {}, threshold = UNDERPERFORM_RATIO) {
  const ctx = peerMedians(panels, serialToGroup);
  return panels.filter(d => isLow(peerRatio(d, ctx, serialToGroup), threshold));
}

// Ratio as a percentage string, or an explicit marker when it is not
// computable. Text — never colour alone — carries this in the table and in each
// map panel's title.
export function formatRatio({ ratio, dark }) {
  if (dark) return '—';
  if (!Number.isFinite(ratio)) return '—';
  return Math.round(ratio * 100) + '%';
}

export function ratioTitle(r) {
  if (r.dark) return 'too dark to compare';
  if (!Number.isFinite(r.ratio)) return 'no comparison available';
  return `${formatRatio(r)} of ${r.group} median`;
}
