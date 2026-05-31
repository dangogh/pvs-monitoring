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
need openssl

GO_VERSION=$(go version | grep -oE 'go[0-9]+\.[0-9]+' | head -1)
echo "Using $GO_VERSION"

# Backup any existing installation before making changes
BACKUP_DEST="${HOME}/backups-pvs"
if [[ -f "$CONFIG_DIR/config.yaml" || -f "${DATA_DIR}/readings.db" ]]; then
    echo "Backing up existing installation to $BACKUP_DEST..."
    bash /tmp/pvs-monitoring/backup.sh -d "$BACKUP_DEST" -s "$DATA_DIR" -c "$CONFIG_DIR" || \
        echo "Warning: backup failed — continuing anyway"
    echo ""
fi

# Clone or update
if [[ -d /tmp/pvs-monitoring ]]; then
    echo "Updating existing clone..."
    git -C /tmp/pvs-monitoring pull --ff-only
else
    echo "Cloning $REPO..."
    git clone "$REPO" /tmp/pvs-monitoring
fi

echo "Building binaries..."
(cd /tmp/pvs-monitoring && go build -o /tmp/pvs-monitor-bin ./cmd/pvs-monitor)
(cd /tmp/pvs-monitoring && go build -o /tmp/pvs-api-bin     ./cmd/pvs-api)
(cd /tmp/pvs-monitoring && go build -o /tmp/pvs-ui-bin      ./cmd/pvs-ui)

echo "Installing to $INSTALL_DIR (may prompt for sudo)..."
sudo install -m 755 /tmp/pvs-monitor-bin "$INSTALL_DIR/pvs-monitor"
sudo install -m 755 /tmp/pvs-api-bin     "$INSTALL_DIR/pvs-api"
sudo install -m 755 /tmp/pvs-ui-bin      "$INSTALL_DIR/pvs-ui"
rm /tmp/pvs-monitor-bin /tmp/pvs-api-bin /tmp/pvs-ui-bin

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

# TLS certificate for pvs-api
TLS_CERT="$DATA_DIR/server.crt"
TLS_KEY="$DATA_DIR/server.key"
if [[ ! -f "$TLS_CERT" || ! -f "$TLS_KEY" ]]; then
    echo "Generating self-signed TLS certificate for pvs-api..."
    openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 \
        -days 3650 -nodes \
        -keyout "$TLS_KEY" -out "$TLS_CERT" \
        -subj "/CN=$(hostname)" \
        2>/dev/null
    chmod 600 "$TLS_KEY"
    echo "Certificate: $TLS_CERT"
    echo "Private key: $TLS_KEY"
    echo ""
    echo "NOTE: This is a self-signed cert. Your browser will show a security warning"
    echo "for pvs-ui. Accept it once (or add it to your trust store) to proceed."
else
    echo "TLS certificate already exists at $TLS_CERT — not regenerated."
fi

# systemd services
if command -v systemctl >/dev/null 2>&1; then
    LOG_FILE="$DATA_DIR/pvs-monitor.log"
    API_LOG_FILE="$DATA_DIR/pvs-api.log"

    # Snapshot which pvs services are currently active before we change anything.
    PVS_SERVICES=(pvs-monitor pvs-api pvs-ui)
    declare -A WAS_ACTIVE
    for svc in "${PVS_SERVICES[@]}"; do
        if systemctl is-active --quiet "$svc" 2>/dev/null; then
            WAS_ACTIVE[$svc]=1
            echo "Stopping $svc..."
            sudo systemctl stop "$svc"
        fi
    done

    echo "Installing systemd services..."

    sudo tee "/etc/systemd/system/pvs-monitor.service" > /dev/null <<EOF
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

    sudo tee "/etc/systemd/system/pvs-api.service" > /dev/null <<EOF
[Unit]
Description=PVS6 solar data API
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=$INSTALL_DIR/pvs-api -tls-cert $TLS_CERT -tls-key $TLS_KEY
Restart=on-failure
RestartSec=10
User=$USER
Environment=HOME=$HOME
StandardOutput=append:$API_LOG_FILE
StandardError=append:$API_LOG_FILE

[Install]
WantedBy=multi-user.target
EOF

    sudo tee "/etc/systemd/system/pvs-ui.service" > /dev/null <<EOF
[Unit]
Description=PVS6 solar dashboard UI
After=pvs-api.service

[Service]
ExecStart=$INSTALL_DIR/pvs-ui -api https://localhost:8081
Restart=on-failure
RestartSec=10
User=$USER
Environment=HOME=$HOME
StandardOutput=append:$DATA_DIR/pvs-ui.log
StandardError=append:$DATA_DIR/pvs-ui.log

[Install]
WantedBy=multi-user.target
EOF

    sudo systemctl daemon-reload
    sudo systemctl enable pvs-monitor pvs-api pvs-ui

    # Restart services that were running before; start any that are newly enabled.
    echo ""
    for svc in "${PVS_SERVICES[@]}"; do
        if [[ -n "${WAS_ACTIVE[$svc]:-}" ]]; then
            echo "Restarting $svc..."
            sudo systemctl start "$svc"
        elif systemctl is-enabled --quiet "$svc" 2>/dev/null; then
            echo "Starting $svc (newly enabled)..."
            sudo systemctl start "$svc"
        fi
    done

    echo ""
    echo "Services installed and running."
    echo "Logs:             tail -f $LOG_FILE"
    echo "                  tail -f $API_LOG_FILE"
    echo "Dashboard:        http://$(hostname -I | awk '{print $1}'):8080  (pvs-ui, plain HTTP)"
    echo "API (HTTPS):      https://$(hostname -I | awk '{print $1}'):8081"
else
    echo ""
    echo "systemd not found — skipping service install."
    echo "Run manually:  pvs-monitor &  pvs-api &  pvs-ui &"
fi

echo ""
echo "Done. Installed to $INSTALL_DIR: pvs-monitor, pvs-api, pvs-ui"
