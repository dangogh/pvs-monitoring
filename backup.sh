#!/usr/bin/env bash
set -euo pipefail

CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/pvs-monitor"
DATA_DIR="${XDG_DATA_HOME:-$HOME/.local/share}/pvs-monitor"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)

usage() {
    cat >&2 <<EOF
Usage: $0 -d DEST [-s SRC_DATA] [-c SRC_CONFIG] [-n]

Options:
  -d DEST        Destination directory (required). A timestamped subdirectory
                 is created inside it: DEST/pvs-backup-YYYYMMDD-HHMMSS/
  -s SRC_DATA    Source data directory (default: $DATA_DIR)
  -c SRC_CONFIG  Source config directory (default: $CONFIG_DIR)
  -n             Dry run — print what would be copied without doing it
  -h             Show this help
EOF
    exit 1
}

SRC_DATA="$DATA_DIR"
SRC_CONFIG="$CONFIG_DIR"
DEST=""
DRY_RUN=0

while getopts "d:s:c:nh" opt; do
    case "$opt" in
        d) DEST="$OPTARG" ;;
        s) SRC_DATA="$OPTARG" ;;
        c) SRC_CONFIG="$OPTARG" ;;
        n) DRY_RUN=1 ;;
        h) usage ;;
        *) usage ;;
    esac
done

if [[ -z "$DEST" ]]; then
    echo "error: -d DEST is required" >&2
    usage
fi

BACKUP_DIR="$DEST/pvs-backup-$TIMESTAMP"

run() {
    if [[ $DRY_RUN -eq 1 ]]; then
        echo "[dry-run] $*"
    else
        "$@"
    fi
}

echo "Backup destination: $BACKUP_DIR"
[[ $DRY_RUN -eq 1 ]] && echo "(dry run — no files will be written)"
echo ""

run mkdir -p "$BACKUP_DIR"

# SQLite database — use the SQLite backup command if sqlite3 is available so
# the copy is safe even if pvs-monitor is writing concurrently.
DB="$SRC_DATA/readings.db"
if [[ -f "$DB" ]]; then
    if command -v sqlite3 >/dev/null 2>&1; then
        echo "Backing up database (sqlite3 online backup)..."
        run sqlite3 "$DB" ".backup '$BACKUP_DIR/readings.db'"
    else
        echo "Backing up database (cp — sqlite3 not found, stop pvs-monitor first for safety)..."
        run cp "$DB" "$BACKUP_DIR/readings.db"
    fi
else
    echo "Warning: database not found at $DB — skipping"
fi

# TLS certificate and key
for f in server.crt server.key; do
    if [[ -f "$SRC_DATA/$f" ]]; then
        echo "Backing up $f..."
        run cp "$SRC_DATA/$f" "$BACKUP_DIR/$f"
    fi
done

# Config file
if [[ -f "$SRC_CONFIG/config.yaml" ]]; then
    echo "Backing up config.yaml..."
    run mkdir -p "$BACKUP_DIR/config"
    run cp "$SRC_CONFIG/config.yaml" "$BACKUP_DIR/config/config.yaml"
else
    echo "Warning: config not found at $SRC_CONFIG/config.yaml — skipping"
fi

if [[ $DRY_RUN -eq 0 ]]; then
    echo ""
    echo "Done. Files written to $BACKUP_DIR:"
    find "$BACKUP_DIR" -type f | sort | sed 's/^/  /'
fi
