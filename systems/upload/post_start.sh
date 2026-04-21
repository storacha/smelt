#!/bin/sh
# Register every piri-N node declared in smelt.yml as a storage provider.
#
# Runs as a Docker Compose post_start hook after upload is healthy. Loops over
# each piri-{N}-proof.txt produced by generate-proofs.sh (or pkg/stack/proofs.go
# in the Go test stack) and adds the corresponding node as a provider with
# equal weight.

set -e

sleep 1  # give the core upload service a moment to settle

registered=0
for proof_file in /proofs/piri-*-proof.txt; do
    [ -f "$proof_file" ] || continue
    node_name=$(basename "$proof_file" -proof.txt)  # piri-0, piri-1, ...
    pub_key="/piri-keys/${node_name}.pub"

    if [ ! -f "$pub_key" ]; then
        echo "post_start: skipping ${node_name} — public key ${pub_key} not found"
        continue
    fi

    proof=$(cat "$proof_file")
    did=$(sprue identity parse "$pub_key")
    endpoint="http://${node_name}:3000"

    echo "post_start: registering ${node_name} (${did}) at ${endpoint}"
    # Tolerate "already registered" — expected when the stack booted from a
    # smelt snapshot that captured upload's dynamodb provider registry. Any
    # other failure is still fatal.
    if add_err=$(sprue client admin provider add "$endpoint" "$proof" 2>&1); then
        :
    elif echo "$add_err" | grep -q "already registered"; then
        echo "post_start:   (${node_name} already registered — continuing)"
    else
        echo "$add_err" >&2
        exit 1
    fi
    sprue client admin provider weight set "$did" 100 100

    registered=$((registered + 1))
done

if [ "$registered" -eq 0 ]; then
    echo "post_start: WARNING — no piri-N nodes were registered"
    echo "post_start:           check that generate-proofs.sh ran and /proofs is populated"
    exit 1
fi

echo "post_start: registered ${registered} piri provider(s)"
