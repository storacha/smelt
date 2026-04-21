#!/bin/bash
# Register every piri-N node declared in smelt.yml as a storage provider.
#
# Runs as a Docker Compose post_start hook after the upload container starts.
# Waits for upload to be healthy, then loops over each piri-{N}-proof.txt
# produced by generate-proofs.sh (or pkg/stack/proofs.go in the Go test stack)
# and adds the corresponding node as a provider with equal weight.

set -e

# Redirect output to the container logs.
exec  > >(sed 's/^/[post_start (OUT)] /' >/proc/1/fd/1) \
     2> >(sed 's/^/[post_start (ERR)] /' >/proc/1/fd/1)

# Wait for sprue to be ready before running admin commands.
# post_start runs after container start, not after healthcheck passes.
echo "waiting for upload service to be ready..."
retries=0
until curl -sf http://localhost:80/health > /dev/null 2>&1; do
    retries=$((retries + 1))
    if [ "$retries" -ge 30 ]; then
        echo "ERROR — upload service not ready after 30 attempts" >&2
        exit 10
    fi
    sleep 2
done

registered=0
for proof_file in /proofs/piri-*-proof.txt; do
    [ -f "$proof_file" ] || continue
    node_name=$(basename "$proof_file" -proof.txt)  # piri-0, piri-1, ...
    pub_key="/piri-keys/${node_name}.pub"

    if [ ! -f "$pub_key" ]; then
        echo "skipping ${node_name} — public key ${pub_key} not found" >&2
        continue
    fi

    proof=$(cat "$proof_file")
    did=$(sprue identity parse "$pub_key")
    endpoint="http://${node_name}:3000"

    echo "registering ${node_name} (${did}) at ${endpoint}"
    sprue client admin provider add "$endpoint" "$proof"
    sprue client admin provider weight set "$did" 100 100

    registered=$((registered + 1))
done

if [ "$registered" -eq 0 ]; then
		{
			echo "WARNING — no piri-N nodes were registered"
			echo "          check that generate-proofs.sh ran and /proofs is populated"
		} >&2
    exit 1
fi

echo "registered ${registered} piri provider(s)"
