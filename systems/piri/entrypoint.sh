#!/bin/bash
# Piri Entrypoint - Initialize and start piri storage node
set -e

# Paths
KEY_FILE="/keys/piri.pem"
WALLET_FILE="/keys/owner-wallet.hex"
BASE_CONFIG="/config/piri-base-config.toml"
DATA_DIR="/data/piri"
TEMP_DIR="/tmp/piri"
CONFIG_FILE="${DATA_DIR}/piri-config.toml"

# Network settings (can be overridden via environment)
LOTUS_ENDPOINT="${LOTUS_ENDPOINT:-ws://blockchain:8545}"
PUBLIC_URL="${PUBLIC_URL:-http://piri:3000}"
PORT="${PORT:-3000}"
HOST="${HOST:-0.0.0.0}"
OPERATOR_EMAIL="${OPERATOR_EMAIL:-local@test.com}"
REGISTRAR_URL="${REGISTRAR_URL:-http://delegator:80}"

echo "=== Piri Entrypoint ==="

# Ensure directories exist
mkdir -p "$DATA_DIR" "$TEMP_DIR"

# Step 1: Extract piri's DID from key file
echo "[1/4] Extracting piri DID..."
PIRI_DID=$(/usr/bin/piri identity parse "$KEY_FILE" 2>&1 | grep -oE 'did:key:z[a-zA-Z0-9]+')
if [ -z "$PIRI_DID" ]; then
    echo "ERROR: Failed to extract DID from $KEY_FILE"
    exit 1
fi
echo "  DID: $PIRI_DID"

# Step 2: Register DID with delegator allow list
echo "[2/4] Registering DID with allow list..."
/scripts/register-did.sh "$PIRI_DID" || echo "Warning: Registration failed, continuing..."

# Step 3: Initialize piri (if not already initialized)
echo "[3/4] Initializing piri..."
if [ -f "$CONFIG_FILE" ] && grep -q "proof_set" "$CONFIG_FILE" 2>/dev/null; then
    echo "  Config exists, skipping init"
else
    [ -f "$CONFIG_FILE" ] && rm -f "$CONFIG_FILE"

    cd "$DATA_DIR"
    /usr/bin/piri init \
        --base-config="$BASE_CONFIG" \
        --registrar-url="$REGISTRAR_URL" \
        --data-dir="$DATA_DIR" \
        --temp-dir="$TEMP_DIR" \
        --key-file="$KEY_FILE" \
        --wallet-file="$WALLET_FILE" \
        --lotus-endpoint="$LOTUS_ENDPOINT" \
        --public-url="$PUBLIC_URL" \
        --port="$PORT" \
        --host="$HOST" \
        --operator-email="$OPERATOR_EMAIL"

    [ -f "piri-config.toml" ] && mv piri-config.toml "$CONFIG_FILE"
    echo "  Init complete"
fi

# Step 4: Start piri server
echo "[4/4] Starting piri..."
exec /usr/bin/piri serve full --config "$CONFIG_FILE" "$@"
