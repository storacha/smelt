#!/bin/sh
# Piri Entrypoint - Initialize and start piri storage node
set -e

# Paths
KEY_FILE="/keys/piri.pem"
WALLET_FILE="/keys/owner-wallet.hex"
BASE_CONFIG="/config/piri-base-config.toml"
OVERRIDES_CONFIG="/config/piri-overrides.toml"
DATA_DIR="/data/piri"
TEMP_DIR="/tmp/piri"
CONFIG_FILE="${DATA_DIR}/piri-config.toml"

# Storage backend config files
DB_POSTGRES_CONFIG="/config/piri-db-postgres.toml"
BLOB_S3_CONFIG="/config/piri-blob-s3.toml"

# Network settings (can be overridden via environment)
LOTUS_ENDPOINT="${LOTUS_ENDPOINT:-ws://blockchain:8545}"
PUBLIC_URL="${PUBLIC_URL:-http://piri:3000}"
PORT="${PORT:-3000}"
HOST="${HOST:-0.0.0.0}"
OPERATOR_EMAIL="${OPERATOR_EMAIL:-local@test.com}"
REGISTRAR_URL="${REGISTRAR_URL:-http://delegator:80}"

# Storage backend selection (independent axes)
DB_BACKEND="${PIRI_DB_BACKEND:-sqlite}"
BLOB_BACKEND="${PIRI_BLOB_BACKEND:-filesystem}"

echo "=== Piri Entrypoint ==="
echo "  Database backend: $DB_BACKEND"
echo "  Blob backend: $BLOB_BACKEND"

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

    # Create merged base config with storage backend settings
    # This ensures piri init uses the correct storage backends from the start
    MERGED_BASE_CONFIG="${TEMP_DIR}/merged-base-config.toml"
    cp "$BASE_CONFIG" "$MERGED_BASE_CONFIG"

    # Append database backend config if postgres
    if [ "$DB_BACKEND" = "postgres" ] && [ -f "$DB_POSTGRES_CONFIG" ]; then
        echo "" >> "$MERGED_BASE_CONFIG"
        echo "# --- database backend: postgres ---" >> "$MERGED_BASE_CONFIG"
        cat "$DB_POSTGRES_CONFIG" >> "$MERGED_BASE_CONFIG"
    fi

    # Append blob storage backend config if s3
    if [ "$BLOB_BACKEND" = "s3" ] && [ -f "$BLOB_S3_CONFIG" ]; then
        echo "" >> "$MERGED_BASE_CONFIG"
        echo "# --- blob backend: s3 ---" >> "$MERGED_BASE_CONFIG"
        cat "$BLOB_S3_CONFIG" >> "$MERGED_BASE_CONFIG"
    fi

    cd "$DATA_DIR"
    /usr/bin/piri init \
        --base-config="$MERGED_BASE_CONFIG" \
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

    # Config created as piri-config.toml in DATA_DIR (current dir)
    echo "  Init complete"

    # Clean up merged config
    rm -f "$MERGED_BASE_CONFIG"
fi

# Append overrides config if present and not already applied
if [ -f "$OVERRIDES_CONFIG" ]; then
    # Check if overrides already appended (look for marker comment)
    if ! grep -q "# --- piri-overrides.toml ---" "$CONFIG_FILE" 2>/dev/null; then
        echo "  Applying config overrides..."
        echo "" >> "$CONFIG_FILE"
        echo "# --- piri-overrides.toml ---" >> "$CONFIG_FILE"
        cat "$OVERRIDES_CONFIG" >> "$CONFIG_FILE"
    fi
fi

# Append overrides config if present and not already applied
if [ -f "$OVERRIDES_CONFIG" ]; then
    # Check if overrides already appended (look for marker comment)
    if ! grep -q "# --- piri-overrides.toml ---" "$CONFIG_FILE" 2>/dev/null; then
        echo "  Applying config overrides..."
        echo "" >> "$CONFIG_FILE"
        echo "# --- piri-overrides.toml ---" >> "$CONFIG_FILE"
        cat "$OVERRIDES_CONFIG" >> "$CONFIG_FILE"
    fi
fi

# Step 4: Start piri server
echo "[4/4] Starting piri..."
exec /usr/bin/piri serve full --config "$CONFIG_FILE" "$@"
