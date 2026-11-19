# PVS Host SD Card Replacement

Complete workflow for replacing a pvs-monitor host's SD card with minimal downtime.

## Phase 1: Before leaving home

### 1. Generate package inventory
```sh
chmod +x inventory-packages.sh
scp inventory-packages.sh $HOST:~/
ssh $HOST "bash ~/inventory-packages.sh"
scp $HOST:~/pvs-host-packages-*.txt ~/backup/
```
Save the package list filename for later. Replace `$HOST` with your target hostname.

### 2. Pre-install new OS
- Use Raspberry Pi Imager (or your preferred tool)
- Install same OS version as current system
- Write to new SD card
- Label the card so you know which one is new

## Phase 2: At the host location (final backup + swap)

### 3. Create database backup
```sh
scp ../backup-db.sh restore-packages.sh $HOST:~/
ssh $HOST "bash ~/backup-db.sh ~"
scp $HOST:~/readings-backup-*.db ~/backup/
```
Save the backup filename. The backup-db.sh script accepts an optional destination directory (defaults to current dir).

### 4. Stop services and shut down
```sh
ssh $HOST "sudo systemctl stop pvs-monitor pvs-api pvs-ui"
ssh $HOST "sudo shutdown -h now"
```
Wait 30 seconds for clean shutdown.

### 5. Physical swap
- Remove power from Raspberry Pi
- Remove old SD card
- Insert new SD card (pre-imaged)
- Power on and wait ~2 minutes for boot
- Verify it comes up

## Phase 3: After returning home (restore)

### 6. Restore packages and database

Copy the backup files to the host:
```sh
scp ~/backup/readings-backup-*.db $HOST:/tmp/
scp ~/backup/pvs-host-packages-*.txt $HOST:~/
```

Restore packages and database:
```sh
ssh $HOST "bash ~/restore-packages.sh ~/pvs-host-packages-*.txt"
ssh $HOST "sudo cp /tmp/readings-backup-*.db /var/lib/pvs-monitor/readings.db"
ssh $HOST "sudo chown pvs-mon:pvs-mon /var/lib/pvs-monitor/readings.db"
```

### 7. Start services
```sh
ssh $HOST "sudo systemctl start pvs-monitor pvs-api pvs-ui && sleep 3"
ssh $HOST "sudo systemctl status pvs-monitor pvs-api pvs-ui"
```

### 8. Verify
```sh
ssh $HOST "df -h"
ssh $HOST "ps aux | grep -E 'pvs-monitor|pvs-api|pvs-ui' | grep -v grep"
ssh $HOST "journalctl -u pvs-monitor -n 3 --no-pager"
```

## Files in this directory

- `inventory-packages.sh` - Capture current package list from old system
- `restore-packages.sh` - Install packages on new system
- `README.md` - This file

Note: `backup-db.sh` has been moved to `scripts/` root as it's a general utility, not just for SD card swaps.

## Notes

- The database backup uses `sqlite3 .backup` which ensures WAL is properly checkpointed
- Package restore gracefully skips packages that don't exist in the new OS version
- All three files (db, WAL, shm) are included in the SQLite backup
- Downtime: ~5 minutes (from service stop to restart on new system)
