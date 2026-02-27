#!/bin/bash
# Generate UCAN delegation proofs for service communication
#
# This script generates the delegation proofs needed for services to communicate:
# - indexing-service-proof: indexer delegates claim/cache to delegator
#
# Prerequisites:
# - Keys must exist in generated/keys/ (run generate-keys.sh first)
# - mkdelegation CLI must be installed (go install github.com/storacha/go-mkdelegation@latest)
#
# Usage: ./generate-proofs.sh [--force]
#   --force: Regenerate all proofs even if they exist

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
KEYS_DIR="$SCRIPT_DIR/keys"
PROOFS_DIR="$SCRIPT_DIR/proofs"
FORCE=${1:-""}

# Check for mkdelegation
MKDELEGATION="${MKDELEGATION:-mkdelegation}"
if ! command -v "$MKDELEGATION" &> /dev/null; then
    echo "Error: mkdelegation not found in PATH"
    echo "Install with: go install github.com/storacha/go-mkdelegation@latest"
    exit 1
fi

# Check for required keys
check_key() {
    local key_file="$1"
    if [[ ! -f "$key_file" ]]; then
        echo "Error: Required key file not found: $key_file"
        echo "Run generate-keys.sh first"
        exit 1
    fi
}

echo "Generating delegation proofs in $PROOFS_DIR..."
echo ""

mkdir -p "$PROOFS_DIR"

# Check required keys exist
check_key "$KEYS_DIR/indexer.pem"
check_key "$KEYS_DIR/delegator.pem"
check_key "$KEYS_DIR/etracker.pem"

# The delegator identifies as did:web:delegator when signing delegations to providers.
# The proofs must have audience=did:web:delegator so the UCAN chain is valid.
DELEGATOR_WEB_DID="did:web:delegator"
echo "Using delegator DID: $DELEGATOR_WEB_DID"

# Generate indexing service proof (indexer → delegator, claim/cache capability)
INDEXING_PROOF_FILE="$PROOFS_DIR/indexing-service-proof.txt"
if [[ -f "$INDEXING_PROOF_FILE" && "$FORCE" != "--force" ]]; then
    echo ""
    echo "[skip] indexing-service-proof.txt already exists"
else
    echo ""
    echo "Generating indexing service proof..."
    echo "  Issuer: did:web:indexer (key: indexer.pem)"
    echo "  Audience: $DELEGATOR_WEB_DID"
    echo "  Capability: claim/cache"

    "$MKDELEGATION" gen \
        --issuer-private-key "$KEYS_DIR/indexer.pem" \
        --issuer-did-web "did:web:indexer" \
        --audience-did-key "$DELEGATOR_WEB_DID" \
        --capabilities "claim/cache" \
        > "$INDEXING_PROOF_FILE"

    echo "  [new] indexing-service-proof.txt"
fi

# Generate egress tracking service proof (etracker → delegator, egress/track capability)
EGRESS_PROOF_FILE="$PROOFS_DIR/egress-tracking-proof.txt"
if [[ -f "$EGRESS_PROOF_FILE" && "$FORCE" != "--force" ]]; then
    echo ""
    echo "[skip] egress-tracking-proof.txt already exists"
else
    echo ""
    echo "Generating egress tracking service proof..."
    echo "  Issuer: did:web:etracker (key: etracker.pem)"
    echo "  Audience: $DELEGATOR_WEB_DID"
    echo "  Capability: egress/track"

    "$MKDELEGATION" gen \
        --issuer-private-key "$KEYS_DIR/etracker.pem" \
        --issuer-did-web "did:web:etracker" \
        --audience-did-key "$DELEGATOR_WEB_DID" \
        --capabilities "egress/track" \
        --skip-capability-validation \
        > "$EGRESS_PROOF_FILE"

    echo "  [new] egress-tracking-proof.txt"
fi

echo ""
echo "Proofs generated in: $PROOFS_DIR"
echo ""
echo "Generated files:"
ls -la "$PROOFS_DIR"/*.txt 2>/dev/null | awk '{print "  " $NF}' || echo "  (none)"
