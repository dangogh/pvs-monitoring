#!/bin/bash
# Inventory packages on current pvs host system
# Run this before SD card swap to capture current state

set -e

OUTFILE="pvs-host-packages-$(date +%Y%m%d-%H%M%S).txt"

echo "Generating package inventory..."

# Get all manually installed packages (not dependencies)
dpkg --get-selections | grep install | awk '{print $1}' > "$OUTFILE"

echo "Inventory saved to: $OUTFILE"
echo "Total packages: $(wc -l < "$OUTFILE")"
cat "$OUTFILE"
