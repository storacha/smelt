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

# PostgreSQL settings (used when DB_BACKEND=postgres)
DB_POSTGRES_URL="${PIRI_DB_POSTGRES_URL:-postgres://piri:piri@piri-postgres:5432/piri?sslmode=disable}"
DB_POSTGRES_MAX_OPEN_CONNS="${PIRI_DB_POSTGRES_MAX_OPEN_CONNS:-10}"
DB_POSTGRES_MAX_IDLE_CONNS="${PIRI_DB_POSTGRES_MAX_IDLE_CONNS:-5}"
DB_POSTGRES_CONN_MAX_LIFETIME="${PIRI_DB_POSTGRES_CONN_MAX_LIFETIME:-30m}"

# S3 settings (used when BLOB_BACKEND=s3)
S3_ENDPOINT="${PIRI_S3_ENDPOINT:-piri-minio:9000}"
S3_BUCKET_PREFIX="${PIRI_S3_BUCKET_PREFIX:-piri-}"
S3_ACCESS_KEY_ID="${PIRI_S3_ACCESS_KEY_ID:-minioadmin}"
S3_SECRET_ACCESS_KEY="${PIRI_S3_SECRET_ACCESS_KEY:-minioadmin}"
S3_INSECURE="${PIRI_S3_INSECURE:-true}"

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

    cd "$DATA_DIR"

    # Build init command with base flags
    INIT_CMD="/usr/bin/piri init \
        --base-config=$BASE_CONFIG \
        --registrar-url=$REGISTRAR_URL \
        --data-dir=$DATA_DIR \
        --temp-dir=$TEMP_DIR \
        --key-file=$KEY_FILE \
        --wallet-file=$WALLET_FILE \
        --lotus-endpoint=$LOTUS_ENDPOINT \
        --public-url=$PUBLIC_URL \
        --port=$PORT \
        --host=$HOST \
        --operator-email=$OPERATOR_EMAIL"

    # Add PostgreSQL flags if postgres backend selected
    if [ "$DB_BACKEND" = "postgres" ]; then
        INIT_CMD="$INIT_CMD \
            --db-type=postgres \
            --db-postgres-url=$DB_POSTGRES_URL \
            --db-postgres-max-open-conns=$DB_POSTGRES_MAX_OPEN_CONNS \
            --db-postgres-max-idle-conns=$DB_POSTGRES_MAX_IDLE_CONNS \
            --db-postgres-conn-max-lifetime=$DB_POSTGRES_CONN_MAX_LIFETIME"
    fi

    # Add S3 flags if s3 backend selected
    if [ "$BLOB_BACKEND" = "s3" ]; then
        INIT_CMD="$INIT_CMD \
            --s3-endpoint=$S3_ENDPOINT \
            --s3-bucket-prefix=$S3_BUCKET_PREFIX \
            --s3-access-key-id=$S3_ACCESS_KEY_ID \
            --s3-secret-access-key=$S3_SECRET_ACCESS_KEY"
        if [ "$S3_INSECURE" = "true" ]; then
            INIT_CMD="$INIT_CMD --s3-insecure"
        fi
    fi

    # Execute the init command
    eval $INIT_CMD

    # Config created as piri-config.toml in DATA_DIR (current dir)
    echo "  Init complete"
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
