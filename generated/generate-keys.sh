#!/bin/bash
# Generate keys for Storacha services
#
# This script generates all cryptographic keys needed for the local devnet:
# - Ed25519 keys in PEM format for UCAN-based services
# - EVM private keys are extracted from the blockchain's deployed-addresses.json
#   (these are pre-funded Anvil/Hardhat accounts)
#
# Usage: ./generate-keys.sh [--force]
#   --force: Regenerate all keys even if they exist

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
KEYS_DIR="$SCRIPT_DIR/keys"
BLOCKCHAIN_STATE="$PROJECT_DIR/systems/blockchain/state/deployed-addresses.json"
FORCE=${1:-""}

# Check for required tools
if ! command -v jq &> /dev/null; then
    echo "Error: jq is required but not installed."
    echo "Install with: sudo apt install jq (Debian/Ubuntu) or brew install jq (macOS)"
    exit 1
fi

echo "Generating service keys in $KEYS_DIR..."
echo ""

mkdir -p "$KEYS_DIR"

# Function to generate an ed25519 key pair in PEM format
generate_ed25519_key() {
    local name="$1"
    local key_file="$KEYS_DIR/$name.pem"

    if [[ -f "$key_file" && "$FORCE" != "--force" ]]; then
        echo "  [skip] $name.pem already exists"
        return
    fi

    # Generate ed25519 private key in PKCS#8 format
    openssl genpkey -algorithm ED25519 -out "$key_file"
    chmod 600 "$key_file"

    # Extract public key for reference
    openssl pkey -in "$key_file" -pubout -out "$KEYS_DIR/$name.pub"

    echo "  [new]  $name.pem"
}

# Function to extract an EVM private key from deployed-addresses.json (raw hex format)
extract_evm_key_raw() {
    local json_path="$1"  # e.g., ".payer.privateKey"
    local output_name="$2"
    local key_file="$KEYS_DIR/$output_name.hex"

    if [[ -f "$key_file" && "$FORCE" != "--force" ]]; then
        echo "  [skip] $output_name.hex already exists"
        return
    fi

    if [[ ! -f "$BLOCKCHAIN_STATE" ]]; then
        echo "  [error] $output_name.hex - blockchain state file not found: $BLOCKCHAIN_STATE"
        return 1
    fi

    # Extract key from JSON, remove 0x prefix
    local private_key
    private_key=$(jq -r "$json_path" "$BLOCKCHAIN_STATE" | sed 's/^0x//')

    if [[ -z "$private_key" || "$private_key" == "null" ]]; then
        echo "  [error] $output_name.hex - could not extract key from $json_path"
        return 1
    fi

    echo "$private_key" > "$key_file"
    chmod 600 "$key_file"

    echo "  [new]  $output_name.hex (from blockchain state)"
}

# Function to extract an EVM private key in piri wallet format
# Format: hex-encoded JSON {"Type":"delegated","PrivateKey":"<base64>"}
extract_piri_wallet() {
    local json_path="$1"  # e.g., ".deployer.privateKey"
    local output_name="$2"
    local key_file="$KEYS_DIR/$output_name.hex"

    if [[ -f "$key_file" && "$FORCE" != "--force" ]]; then
        echo "  [skip] $output_name.hex already exists"
        return
    fi

    if [[ ! -f "$BLOCKCHAIN_STATE" ]]; then
        echo "  [error] $output_name.hex - blockchain state file not found: $BLOCKCHAIN_STATE"
        return 1
    fi

    # Extract key from JSON, remove 0x prefix
    local private_key_hex
    private_key_hex=$(jq -r "$json_path" "$BLOCKCHAIN_STATE" | sed 's/^0x//')

    if [[ -z "$private_key_hex" || "$private_key_hex" == "null" ]]; then
        echo "  [error] $output_name.hex - could not extract key from $json_path"
        return 1
    fi

    # Convert hex to binary, then to base64
    local private_key_base64
    private_key_base64=$(echo "$private_key_hex" | xxd -r -p | base64 -w 0)

    # Create JSON structure and hex-encode it
    local json_content="{\"Type\":\"delegated\",\"PrivateKey\":\"${private_key_base64}\"}"
    echo -n "$json_content" | xxd -p | tr -d '\n' > "$key_file"
    chmod 600 "$key_file"

    echo "  [new]  $output_name.hex (piri wallet format)"
}

echo "Ed25519 keys (PEM format):"
generate_ed25519_key "piri"
generate_ed25519_key "upload"
generate_ed25519_key "indexer"
generate_ed25519_key "delegator"
generate_ed25519_key "signing-service"
generate_ed25519_key "etracker"

echo ""
echo "EVM keys (from blockchain pre-funded accounts):"
extract_evm_key_raw ".payer.privateKey" "payer-key"    # Raw hex, used by signing-service
extract_piri_wallet ".deployer.privateKey" "owner-wallet"  # Piri wallet format

echo ""
echo "Keys generated in: $KEYS_DIR"
echo ""

# List all generated keys
echo "Generated files:"
ls -la "$KEYS_DIR"/*.pem "$KEYS_DIR"/*.hex 2>/dev/null | awk '{print "  " $NF}' || true

echo ""
echo "Notes:"
echo "  - Ed25519 keys: View DID with 'piri identity parse --key-file <file>'"
echo "  - EVM keys: Extracted from $BLOCKCHAIN_STATE"
echo "    - payer-key.hex: Used by signing-service for blockchain payments"
echo "    - owner-wallet.hex: Used by piri for provider registration"
