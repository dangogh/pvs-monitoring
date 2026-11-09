#!/bin/bash
# Restore packages on new pvs host system
# Usage: ./restore-packages.sh <package-list-file>

set -e

if [[ ! -f "$1" ]]; then
  echo "Usage: $0 <package-list-file>"
  echo "Example: $0 pvs-host-packages-20261108-143000.txt"
  exit 1
fi

PKGFILE="$1"

echo "Updating apt cache..."
sudo apt-get update

echo "Installing packages from $PKGFILE..."

INSTALLED=0
NOTFOUND=0
FAILED=0

while IFS= read -r pkg; do
  if [[ -z "$pkg" ]] || [[ "$pkg" == "#"* ]]; then
    continue
  fi

  # Try to install; if not found, note it and continue
  if sudo apt-get install -y "$pkg" 2>/dev/null; then
    ((INSTALLED++))
  else
    echo "⚠️  Package not found or failed: $pkg"
    ((NOTFOUND++))
  fi
done < "$PKGFILE"

echo ""
echo "=========================================="
echo "Install complete:"
echo "  Installed: $INSTALLED"
echo "  Not found/failed: $NOTFOUND"
echo "=========================================="

if [[ $NOTFOUND -gt 0 ]]; then
  echo ""
  echo "Some packages couldn't be installed (likely removed or renamed)."
  echo "Review the warnings above. Critical services should be fine."
fi
