#!/bin/sh
# pvs-heartbeat.sh — dead-man switch for pvs-monitor.
#
# Pings healthchecks.io only while BOTH data streams are fresh. If pings stop,
# or an explicit /fail is sent, healthchecks.io alerts.
#
# Two streams are checked because they fail independently, as 2026-09-22 proved:
# the WebSocket power stream (`readings`) went dead for 58 hours while the device
# poller (`aux_device_readings`) carried on without a hiccup. Watching only one
# of them is watching half the failure class.
#
# Run by pvs-heartbeat.timer every 5 minutes. All output goes to the journal —
# never redirect it to /dev/null. The reason this incident went unnoticed for 58
# hours was not detection, which worked on the first try; it was that nothing
# anywhere recorded whether the alert had actually been sent.
set -eu

CONF=${PVS_HEARTBEAT_CONF:-/etc/pvs-monitor/heartbeat.conf}
DB=${PVS_HEARTBEAT_DB:-/var/lib/pvs-monitor/readings.db}

# Source the config BEFORE applying defaults, or every value it sets is silently
# overwritten by the default that follows.
[ -r "$CONF" ] && . "$CONF"

PING_BASE=${HEARTBEAT_PING_BASE:-https://hc-ping.com}
# Newest readings row must be younger than this (1 Hz stream, so 15 min is
# already 900 missed samples).
READINGS_MAX_AGE=${READINGS_MAX_AGE:-900}
# The device poller runs once a minute, so it gets a looser bound. Keeping them
# separate means raising the poll interval later cannot start false-alarming.
AUX_MAX_AGE=${AUX_MAX_AGE:-1800}

# No check configured: this host does not want a dead-man switch. Exit quietly so
# the timer does not accumulate failed units on a machine that never opted in.
if [ -z "${HEARTBEAT_UUID:-}" ]; then
	exit 0
fi

URL="$PING_BASE/$HEARTBEAT_UUID"

# ping sends to the check, appending the given path (empty for success).
# --retry covers a transient network blip; failure to reach healthchecks.io is
# reported rather than swallowed, since a silent ping failure is what hid the
# September outage.
ping() {
	if curl -fsS -m 10 --retry 3 "$URL$1" >/dev/null; then
		return 0
	fi
	echo "pvs-heartbeat: WARNING could not reach $PING_BASE (ping '$1' failed)"
	return 1
}

# age_of prints the age in seconds of the newest row in the given table, or
# nothing if it cannot be determined. The `|| true` matters: under `set -e` a
# failing sqlite3 would abort the script here, and the script aborting is the one
# outcome that sends no /fail ping at all — silence, from the component whose
# entire job is to break silence.
age_of() {
	sqlite3 "$DB" "SELECT CAST(strftime('%s','now') AS INTEGER) - MAX(received_at) FROM $1;" 2>/dev/null || true
}

readings_age=$(age_of readings)
aux_age=$(age_of aux_device_readings)

for age in "$readings_age" "$aux_age"; do
	case "$age" in
	"" | *[!0-9-]*)
		echo "pvs-heartbeat: cannot read $DB — failing the check"
		ping /fail || true
		exit 0
		;;
	esac
done

stale=""
if [ "$readings_age" -gt "$READINGS_MAX_AGE" ]; then
	stale="readings ${readings_age}s > ${READINGS_MAX_AGE}s"
fi
if [ "$aux_age" -gt "$AUX_MAX_AGE" ]; then
	stale="${stale:+$stale; }aux_device_readings ${aux_age}s > ${AUX_MAX_AGE}s"
fi

if [ -n "$stale" ]; then
	echo "pvs-heartbeat: STALE $stale — failing the check"
	ping /fail || true
else
	echo "pvs-heartbeat: ok (readings ${readings_age}s, aux ${aux_age}s)"
	ping "" || true
fi
