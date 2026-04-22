#!/bin/sh
# Register every piri-N node declared in smelt.yml as a storage provider.
#
# Runs as a Docker Compose post_start hook after upload is healthy. Loops over
# each piri-{N}-proof.txt produced by generate-proofs.sh (or pkg/stack/proofs.go
# in the Go test stack) and adds the corresponding node as a provider with
# equal weight.

set -e

# Compose routes post_start stdout/stderr somewhere that neither `docker logs`
# nor testcontainers-go's compose client surfaces. Redirect to PID 1's
# stdout/stderr so everything this script prints shows up in `docker logs
# <upload>` — including the captured error before `exit 1`, which is otherwise
# lost and makes CI failures opaque.
exec > /proc/1/fd/1 2> /proc/1/fd/2

# Wait for sprue's HTTP server to actually be listening before making admin
# calls. post_start fires when the container's main process starts, not when
# it's ready — sprue's fx DI takes a few seconds locally, and tens of seconds
# on a loaded CI runner. The previous `sleep 1` here was a race: under CI load
# the CLI below hit connection-refused before sprue bound port 80, the script
# exited 1, and compose killed the container.
echo "post_start: waiting for sprue to start serving on :80..."
waited=0
until curl -sf http://localhost:80/health >/dev/null 2>&1; do
    if [ "$waited" -ge 120 ]; then
        echo "post_start: sprue never started serving after ${waited}s — aborting" >&2
        exit 1
    fi
    sleep 1
    waited=$((waited + 1))
done
echo "post_start: sprue is serving (took ${waited}s)"

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
