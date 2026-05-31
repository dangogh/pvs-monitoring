#!/usr/bin/env bash
set -euo pipefail

REPO="https://github.com/dangogh/pvs-monitoring"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/pvs-monitor"
DATA_DIR="${XDG_DATA_HOME:-$HOME/.local/share}/pvs-monitor"
SERVICE_NAME="pvs-monitor"

need() {
    command -v "$1" >/dev/null 2>&1 || { echo "error: $1 is required but not installed"; exit 1; }
}

need git
need go

GO_VERSION=$(go version | grep -oE 'go[0-9]+\.[0-9]+' | head -1)
echo "Using $GO_VERSION"

# Clone or update
if [[ -d /tmp/pvs-monitoring ]]; then
    echo "Updating existing clone..."
    git -C /tmp/pvs-monitoring pull --ff-only
else
    echo "Cloning $REPO..."
    git clone "$REPO" /tmp/pvs-monitoring
fi

echo "Building pvs-monitor..."
(cd /tmp/pvs-monitoring && go build -o /tmp/pvs-monitor-bin ./cmd/pvs-monitor)

echo "Installing to $INSTALL_DIR/pvs-monitor (may prompt for sudo)..."
sudo install -m 755 /tmp/pvs-monitor-bin "$INSTALL_DIR/pvs-monitor"
rm /tmp/pvs-monitor-bin

# Config
mkdir -p "$CONFIG_DIR" "$DATA_DIR"
if [[ ! -f "$CONFIG_DIR/config.yaml" ]]; then
    cp /tmp/pvs-monitoring/config.example.yaml "$CONFIG_DIR/config.yaml"
    echo ""
    echo "Config created at $CONFIG_DIR/config.yaml"
    echo "Edit it to set your PVS6 address and password before starting the service."
else
    echo "Config already exists at $CONFIG_DIR/config.yaml — not overwritten."
fi

# systemd service
if command -v systemctl >/dev/null 2>&1; then
    UNIT_FILE="/etc/systemd/system/${SERVICE_NAME}.service"
    LOG_FILE="$DATA_DIR/pvs-monitor.log"

    echo "Installing systemd service..."
    sudo tee "$UNIT_FILE" > /dev/null <<EOF
[Unit]
Description=PVS6 solar monitor
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=$INSTALL_DIR/pvs-monitor
Restart=on-failure
RestartSec=30
User=$USER
Environment=HOME=$HOME
StandardOutput=append:$LOG_FILE
StandardError=append:$LOG_FILE

[Install]
WantedBy=multi-user.target
EOF

    sudo systemctl daemon-reload
    sudo systemctl enable "$SERVICE_NAME"
    echo ""
    echo "Service installed and enabled."
    echo "Start it with:  sudo systemctl start $SERVICE_NAME"
    echo "Logs:           tail -f $LOG_FILE"
else
    echo ""
    echo "systemd not found — skipping service install."
    echo "Run manually:  pvs-monitor"
fi

echo ""
echo "Done. pvs-monitor installed to $INSTALL_DIR/pvs-monitor"
