#!/bin/bash
# Generate UCAN delegation proofs for service communication
#
# This script generates the delegation proofs needed for services to communicate:
# - indexing-service-proof: indexer delegates claim/cache to delegator
#
# Prerequisites:
# - Keys must exist in generated/keys/ (run 'make generate' first)
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
    MKDELEGATION="go-mkdelegation"
    if ! command -v "$MKDELEGATION" &> /dev/null; then
        echo "Error: mkdelegation not found in PATH"
        echo "Install with: go install github.com/storacha/go-mkdelegation@latest"
        exit 1
    fi
fi

# Check for required keys
check_key() {
    local key_file="$1"
    if [[ ! -f "$key_file" ]]; then
        echo "Error: Required key file not found: $key_file"
        echo "Run 'make generate' first"
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
# Per-node piri keys (piri-0.pem, piri-1.pem, ...) are checked by the loop below.

# The delegator identifies as did:web:delegator when signing delegations to providers.
# The proofs must have audience=did:web:delegator so the UCAN chain is valid.
DELEGATOR_WEB_DID="did:web:delegator"
echo "Using delegator DID: $DELEGATOR_WEB_DID"

UPLOAD_WEB_DID="did:web:upload"
echo "Using upload DID: $UPLOAD_WEB_DID"

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
    echo "  Capabilities: claim/cache"

    "$MKDELEGATION" gen \
        --issuer-private-key-file "$KEYS_DIR/indexer.pem" \
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
    echo "  Capabilities: egress/track"

    "$MKDELEGATION" gen \
        --issuer-private-key-file "$KEYS_DIR/etracker.pem" \
        --issuer-did-web "did:web:etracker" \
        --audience-did-key "$DELEGATOR_WEB_DID" \
        --capabilities "egress/track" \
        --skip-capability-validation \
        > "$EGRESS_PROOF_FILE"

    echo "  [new] egress-tracking-proof.txt"
fi

# Generate per-node piri proofs (piri-N → upload, blob/* + pdp/info capabilities).
# Loops over every piri-{N}.pem emitted by `smelt generate`, producing one
# delegation per node at $PROOFS_DIR/piri-{N}-proof.txt. Upload's post_start.sh
# consumes these to register each node as a separate storage provider.
PIRI_KEYS_FOUND=0
for PIRI_KEY in "$KEYS_DIR"/piri-*.pem; do
    [[ -f "$PIRI_KEY" ]] || continue
    NODE_NAME=$(basename "$PIRI_KEY" .pem)
    # Accept only piri-<N> (skip things like piri-signing-service.pem).
    [[ "$NODE_NAME" =~ ^piri-[0-9]+$ ]] || continue
    PIRI_KEYS_FOUND=1

    PIRI_PROOF_FILE="$PROOFS_DIR/${NODE_NAME}-proof.txt"
    if [[ -f "$PIRI_PROOF_FILE" && "$FORCE" != "--force" ]]; then
        echo ""
        echo "[skip] ${NODE_NAME}-proof.txt already exists"
        continue
    fi

    echo ""
    echo "Generating ${NODE_NAME} proof..."
    echo "  Issuer: ${NODE_NAME}.pem"
    echo "  Audience: $UPLOAD_WEB_DID"
    echo "  Capabilities: blob/allocate, blob/accept, blob/replica/allocate, pdp/info"

    "$MKDELEGATION" gen \
        --issuer-private-key-file "$PIRI_KEY" \
        --audience-did-key "$UPLOAD_WEB_DID" \
        --capabilities "blob/allocate" \
        --capabilities "blob/accept" \
        --capabilities "blob/replica/allocate" \
        --capabilities "pdp/info" \
        --skip-capability-validation \
        > "$PIRI_PROOF_FILE"

    echo "  [new] ${NODE_NAME}-proof.txt"
done

if [[ "$PIRI_KEYS_FOUND" -eq 0 ]]; then
    echo ""
    echo "WARNING: No piri-N.pem keys found in $KEYS_DIR — skipping piri proofs."
    echo "         Run 'make generate' to create them."
fi

echo ""
echo "Proofs generated in: $PROOFS_DIR"
echo ""
echo "Generated files:"
ls -la "$PROOFS_DIR"/*.txt 2>/dev/null | awk '{print "  " $NF}' || echo "  (none)"
