#!/bin/bash
# IPNI Entrypoint - Initialize and start storetheindex
set -e

DATA_DIR="/root/.storetheindex"

# Initialize if not already done
if [ ! -f "$DATA_DIR/config" ]; then
    echo "Initializing storetheindex..."
    /usr/local/bin/storetheindex init
    echo "Init complete"
else
    echo "Config exists, skipping init"
fi

# Start daemon
echo "Starting storetheindex daemon..."
exec /usr/local/bin/storetheindex daemon "$@"
