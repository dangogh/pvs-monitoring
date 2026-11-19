#!/bin/bash
# Backup pvs-monitor database using sqlite3 backup command
# Ensures WAL is properly checkpointed and backup is consistent
#
# Usage: ./backup-db.sh [backup-dir]
# Default backup-dir: /tmp

set -e

DBFILE="/var/lib/pvs-monitor/readings.db"
BACKUP_DIR="${1:-.}"
BACKUPFILE="$BACKUP_DIR/readings-backup-$(date +%Y%m%d-%H%M%S).db"

if [[ ! -d "$BACKUP_DIR" ]]; then
  echo "Error: backup directory does not exist: $BACKUP_DIR"
  exit 1
fi

echo "Creating SQLite backup to $BACKUPFILE..."

sudo sqlite3 "$DBFILE" ".backup '$BACKUPFILE'"

echo "Backup complete: $BACKUPFILE"
echo "File size: $(ls -lh "$BACKUPFILE" | awk '{print $5}')"
