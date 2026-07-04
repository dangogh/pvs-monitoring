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

# Currently installed version (empty string if not installed)
installed=$(dpkg-query -W -f='${Version}' "$PKG" 2>/dev/null || true)

if [[ "$installed" == "$latest_ver" ]]; then
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

log "Installing..."
sudo dpkg -i "$tmp"

log "Done — ${PKG} ${latest_ver} installed"
