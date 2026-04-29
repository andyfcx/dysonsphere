#!/bin/sh
# Deploy and enroll observer-agent on a remote host.
# Usage: deploy-agent.sh <ssh-host> <server-url> <enrollment-token>
set -e

SSH_HOST="$1"
SERVER_URL="$2"
ENROLL_TOKEN="$3"

if [ -z "$SSH_HOST" ] || [ -z "$SERVER_URL" ] || [ -z "$ENROLL_TOKEN" ]; then
  echo "Usage: $0 <ssh-host> <server-url> <enrollment-token>"
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BINARY="$SCRIPT_DIR/../bin/observer-agent-linux-amd64"
SERVICE_FILE="$SCRIPT_DIR/systemd/observer-agent.service"

if [ ! -f "$BINARY" ]; then
  echo "Binary not found: $BINARY — build it first"
  exit 1
fi

echo "==> [$SSH_HOST] Copying binary..."
scp "$BINARY" "$SSH_HOST:/usr/local/bin/observer-agent"
ssh "$SSH_HOST" "chmod +x /usr/local/bin/observer-agent"

echo "==> [$SSH_HOST] Creating directories..."
ssh "$SSH_HOST" "mkdir -p /etc/observer-agent /var/lib/observer-agent"

echo "==> [$SSH_HOST] Running observer-agent init..."
ssh "$SSH_HOST" "/usr/local/bin/observer-agent init \
  --server '$SERVER_URL' \
  --enrollment-token '$ENROLL_TOKEN' \
  --config-out /etc/observer-agent/config.yaml \
  --state-file /var/lib/observer-agent/state.db \
  --token-file /etc/observer-agent/credentials.yaml"

echo "==> [$SSH_HOST] Installing systemd service..."
scp "$SERVICE_FILE" "$SSH_HOST:/etc/systemd/system/observer-agent.service"
ssh "$SSH_HOST" "systemctl daemon-reload && systemctl enable --now observer-agent"

echo ""
echo "==> [$SSH_HOST] Done! Service status:"
ssh "$SSH_HOST" "systemctl status observer-agent --no-pager -l"
