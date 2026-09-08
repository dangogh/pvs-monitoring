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
//
// Chosen by replaying 27 days (2026-08-10..09-05) at 10-minute cadence: at 0.10
// the shoulder hours produced 34 false alarms, every one of them around 08:30
// when shadows sweep across a group and its members legitimately disagree. 0.15
// removes all of them and still leaves ~79% of the previously-usable daylight
// ticks, with the 08-23 branch outage detected at the same minute.
export const LOW_LIGHT_KW = 0.15;

// Below this fraction of the peer median a panel is flagged as underperforming.
export const UNDERPERFORM_RATIO = 0.60;

export const UNGROUPED = 'ungrouped';

// A median needs peers to be a median. Below this many panels in a group the
// comparison is against too few others to mean anything — at a group of one it
// is the panel against itself, which reads 100% forever and can never be
// flagged. Report no comparison instead of a reassuring number.
export const MIN_PEERS = 3;

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
  const medians = {}, counts = {};
  Object.keys(byGroup).forEach(g => {
    medians[g] = median(byGroup[g]);
    counts[g] = byGroup[g].length;
  });
  return { medians, counts, fleet: median(panels.map(d => d.power_kw)) };
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
  // Too few peers to compare against: not a low-light condition, just no
  // usable denominator. Falls through to the "no comparison" rendering.
  if ((ctx.counts?.[group] ?? 0) < MIN_PEERS) return { ratio: NaN, dark: false, group };
  const med = ctx.medians[group];
  // A group median of zero while the array is producing means the whole group
  // is out — a tripped branch, the most severe thing this view can show. The
  // median cannot express it (it has followed the group to zero), so report it
  // directly instead of rendering "no data" over an entire dead branch.
  if (!(med > 0)) return { ratio: 0, dark: false, group, groupDead: true };
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
  if (!Number.isFinite(r.ratio)) return `too few peers in ${r.group} to compare`;
  if (r.groupDead) return `every panel in ${r.group} is offline`;
  return `${formatRatio(r)} of ${r.group} median`;
}

// The part of the ratio a sighted reader gets from position and colour: which
// group the comparison is against, and whether it counts as underperforming.
// Rendered as visually-hidden text so a screen reader hears it too — a `title`
// attribute is not reliably announced, and the red/bullet cue is decoration.
export function ratioSrText(r, threshold = UNDERPERFORM_RATIO) {
  if (r.dark) return 'too dark to compare';
  if (!Number.isFinite(r.ratio)) return 'too few peers to compare';
  if (r.groupDead) return `, every panel in ${r.group} is offline`;
  return ` of ${r.group} median` + (isLow(r, threshold) ? ', underperforming' : '');
}
