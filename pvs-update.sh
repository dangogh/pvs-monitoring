#!/usr/bin/env bash
# pvs-update.sh — check for a new pvs-monitoring release and install it if found.
# Run daily via cron: 0 6 * * * /usr/local/bin/pvs-update.sh
set -euo pipefail

REPO="dangogh/pvs-monitoring"
PKG="pvs-monitoring"

log() { echo "$(date '+%Y-%m-%d %H:%M:%S') $*"; }

# Latest release tag from GitHub API
latest=$(curl -sf "https://api.github.com/repos/${REPO}/releases/latest" \
  | jq -r '.tag_name')

if [[ -z "$latest" || "$latest" == "null" ]]; then
  log "ERROR: could not fetch latest release from GitHub"
  exit 1
fi

# Strip leading 'v' to match dpkg version format
latest_ver="${latest#v}"

# Currently installed version and dpkg state (empty strings if not installed).
# The state matters as much as the version: a package that failed to configure
# is left "unpacked" with the NEW version already recorded, so comparing
# versions alone would report it as up to date forever while the running
# services stay on the old binaries. Repair that case instead of skipping it.
installed=$(dpkg-query -W -f='${Version}' "$PKG" 2>/dev/null || true)
state=$(dpkg-query -W -f='${db:Status-Status}' "$PKG" 2>/dev/null || true)

if [[ -n "$installed" && "$state" != "installed" ]]; then
  log "WARNING: ${PKG} ${installed} is in dpkg state '${state}', not 'installed' — repairing"
  if sudo DEBIAN_FRONTEND=noninteractive dpkg --force-confdef --force-confold \
       --configure "$PKG" </dev/null; then
    log "Repaired — ${PKG} ${installed} now configured"
    state=$(dpkg-query -W -f='${db:Status-Status}' "$PKG" 2>/dev/null || true)
  else
    log "ERROR: could not configure ${PKG}; manual intervention needed"
    exit 1
  fi
fi

if [[ "$installed" == "$latest_ver" && "$state" == "installed" ]]; then
  log "Already at latest version ${latest_ver} — nothing to do"
  exit 0
fi

log "Updating ${PKG}: ${installed:-not installed} → ${latest_ver}"

deb_url=$(curl -sf "https://api.github.com/repos/${REPO}/releases/latest" \
  | jq -r '.assets[] | select(.name | endswith(".deb")) | .browser_download_url')

if [[ -z "$deb_url" ]]; then
  log "ERROR: no .deb asset found in release ${latest}"
  exit 1
fi

tmp=$(mktemp /tmp/pvs-update-XXXXXX.deb)
trap 'rm -f "$tmp"' EXIT

log "Downloading ${deb_url}"
curl -sfL "$deb_url" -o "$tmp"

# This runs unattended from cron, so it must never reach an interactive prompt.
# Redirecting stdin from /dev/null alone is not enough — dpkg treats EOF at a
# conffile prompt as a fatal error — so also pre-answer conffile questions by
# keeping the installed version of any config the admin has modified.
log "Installing..."
sudo DEBIAN_FRONTEND=noninteractive dpkg --force-confdef --force-confold \
  -i "$tmp" </dev/null

# dpkg -i exits 0 in some partial-failure cases, so confirm the end state
# rather than trusting the exit code.
state=$(dpkg-query -W -f='${db:Status-Status}' "$PKG" 2>/dev/null || true)
if [[ "$state" != "installed" ]]; then
  log "ERROR: ${PKG} left in dpkg state '${state}' after install — services may still be running old binaries"
  exit 1
fi

log "Done — ${PKG} ${latest_ver} installed"
