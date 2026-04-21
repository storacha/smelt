# Getting Started with Smelt

This guide walks you through setting up Smelt—a complete Storacha network running on your laptop—from first clone to first upload. It explains what happens at each step, because developers who understand their tools make fewer trips to Stack Overflow.

---

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Installation](#installation)
3. [Understanding the Setup Process](#understanding-the-setup-process)
4. [Starting the Network](#starting-the-network)
5. [Verifying Health](#verifying-health)
6. [Your First Upload](#your-first-upload)
7. [Your First Retrieval](#your-first-retrieval)
8. [Exploring the Services](#exploring-the-services)
9. [Common First-Time Issues](#common-first-time-issues)
10. [Stopping and Cleaning Up](#stopping-and-cleaning-up)
11. [Next Steps](#next-steps)

---

## Prerequisites

Before you begin, ensure your system has the following:

### Docker Engine 25+ and Docker Compose

Smelt requires docker engine 25.0 or newer. Older engines silently ignore
`healthcheck.start_interval` (resulting in slower boots) and don't support
the compose top-level `name:` key (breaking snapshot portability across
checkouts). The Makefile's `check-docker` target fails early with an upgrade
pointer on older engines.

Docker Compose V2 is required (the `docker compose` subcommand, not the
legacy `docker-compose` binary). Most Docker installations from 2024 onward
include both.

```bash
# Verify your installation
docker version --format '{{.Server.Version}}'    # Should be 25.0 or higher
docker compose version                            # Should show "Docker Compose version v2.x.x"
```

If `docker compose` fails but `docker-compose` works, you have the legacy
version. Upgrade Docker Desktop or install the compose plugin separately.

### Go 1.22+

Go is required for two things:

1. **`smelt generate`** — the multi-piri manifest generator that reads `smelt.yml` and produces `generated/compose/piri.yml` plus all service keys. Runs automatically via `make generate` (and transitively from `make up`, `make init`, `make fresh`).
2. **`mkdelegation`** — creates the UCAN delegation proofs that let services authorize each other. Installed on first `make init`.

```bash
go version    # Should show "go1.22" or higher
```

If Go is unavailable, `make generate` will fail outright (it's required) and `mkdelegation` will be skipped with a warning. Install Go before proceeding.

### jq

The key generation scripts use `jq` to parse JSON configuration files.

```bash
# macOS
brew install jq

# Debian/Ubuntu
sudo apt install jq

# Verify
jq --version
```

### Disk Space

Reserve approximately 5GB for Docker images and data volumes. The initial image pull downloads several custom-built service images, and the blockchain state requires room to grow.

### Operating System

Smelt runs on:

- **macOS**: Intel or Apple Silicon
- **Linux**: Any distribution with Docker support

Windows (including WSL2) is not a supported development host.

---

## Installation

Clone the repository and enter the directory:

```bash
git clone https://github.com/storacha/smelt.git
cd smelt
```

That's it. The actual initialization happens when you first start the network.

---

## Configuring Piri Nodes (Optional)

Smelt ships with a single-piri default, so you can skip this section on your first run. When you want to explore multi-provider scenarios, edit `smelt.yml` at the repo root:

```yaml
version: 1
piri:
  nodes:
    - storage: { db: sqlite,   blob: filesystem }  # piri-0 (default)
    # Uncomment additional nodes to run multi-provider setups:
    # - storage: { db: postgres, blob: filesystem }  # piri-1
    # - storage: { db: sqlite,   blob: s3 }          # piri-2
    # - storage: { db: postgres, blob: s3 }          # piri-3
```

Each entry becomes a `piri-{N}` container exposed on host port `4000 + N`. You can mix and match storage backends per node. Up to 9 nodes total (limited by Anvil's pre-funded accounts). Shared `piri-postgres` and `piri-minio` services are included automatically when any node uses those backends.

See [docs/MULTI_PIRI.md](MULTI_PIRI.md) for the full schema, database namespacing, hot-add/remove behavior, and Anvil wallet mapping. If you edit `smelt.yml` while the network is running, `make up` picks up the change (adding new nodes and `--remove-orphans` removing deleted ones).

---

## Understanding the Setup Process

When you run `make up` for the first time (or `make init` explicitly), the system prepares the environment through several distinct phases. Understanding these phases helps when something goes wrong—and something always goes wrong eventually.

### What `make init` Does

The initialization script (`scripts/init.sh`) performs five steps:

#### Step 1: Create Directory Structure

```
generated/
  keys/               # Cryptographic keys for service identities
  proofs/             # UCAN delegation proofs for service authorization
  compose/            # Generated Docker Compose files (piri.yml from smelt.yml)
  snapshot-scratch/   # Working chain state + session manifest when a snapshot is loaded
  snapshots/          # Personal snapshots saved via `smelt snapshot save`
```

The `generated/` directory is gitignored in full. Shared/team snapshots
live under `snapshots/` at the project root instead — see
[SNAPSHOTS.md](SNAPSHOTS.md). Your keys are local to your machine.

#### Step 2: Generate Ed25519 Keypairs

`smelt generate` (invoked by `make generate`) produces PEM-format Ed25519 keys for every service that needs a cryptographic identity:

| Key File | Service | Purpose |
|----------|---------|---------|
| `piri-0.pem` | First piri node | Signs storage commitments and content claims |
| `piri-{N}.pem` | Additional piri nodes declared in `smelt.yml` | Per-node identities (up to piri-8) |
| `upload.pem` | Upload service | Signs upload coordination messages |
| `indexer.pem` | Indexer | Signs index claims |
| `delegator.pem` | Delegator | Issues capability delegations |
| `signing-service.pem` | Signing service | Signs PDP blockchain operations |
| `etracker.pem` | Egress tracker | Signs egress tracking claims |

Each key maps to a `did:key` identifier. For example, `piri-0.pem` might produce `did:key:z6MkfYoQ6dppqssZ9qHF6PbBzCjoS1wWg15GYxNaMiLZn5RD`. These identifiers appear throughout logs and error messages. Key generation is idempotent — existing keys are preserved on subsequent runs, so adding a node to `smelt.yml` allocates a new key without disturbing existing ones.

#### Step 3: Assign EVM Wallets from Anvil

Anvil ships with 10 deterministic pre-funded accounts. `smelt generate` assigns them to services:

| Key File | Anvil Account | Purpose |
|----------|---------------|---------|
| `payer-key.hex` | Account 1 | Signing-service payer (pays gas for PDP operations) |
| `piri-0-wallet.hex` | Account 0 (deployer) | First piri node's on-chain identity |
| `piri-1-wallet.hex` | Account 2 | Second piri node (if declared) |
| `piri-{N}-wallet.hex` | Account N + 1 | N-th piri node (for N ≥ 1); max is piri-8 → account 9 |

Wallets are generated alongside the corresponding Ed25519 keys, so adding nodes in `smelt.yml` allocates new accounts sequentially. Account 1 is reserved for the payer, which is why piri-1 uses account 2, not account 1.

#### Step 4: Install mkdelegation Tool

If Go is available, the script installs `mkdelegation`:

```bash
go install github.com/storacha/go-mkdelegation@latest
```

This tool generates UCAN delegation proofs—signed statements that grant one service permission to invoke capabilities on another.

#### Step 5: Generate UCAN Delegation Proofs

Two proofs are generated:

**Indexing Service Proof** (`indexing-service-proof.txt`)
- Issuer: `did:web:indexer`
- Audience: `did:web:delegator`
- Capability: `claim/cache`

This proof allows the delegator to cache claims with the indexer on behalf of storage providers.

**Egress Tracking Proof** (`egress-tracking-proof.txt`)
- Issuer: `did:web:etracker`
- Audience: `did:web:delegator`
- Capability: `egress/track`

This proof enables egress tracking functionality through the delegator.

#### Step 6: Create Docker Network

Finally, the script creates the `storacha-network` Docker network:

```bash
docker network create storacha-network
```

All services attach to this network, enabling them to reach each other by container name (e.g., `http://piri:3000` from within the upload service).

---

## Starting the Network

With prerequisites in place:

```bash
make up
```

This command:

1. Runs `make init` if the `generated/keys/` directory is empty
2. Starts all services via `docker compose up -d`
3. Returns immediately (services start in background)

### What Happens During Startup

Docker Compose starts ten services with dependency ordering. Services wait for their dependencies to become healthy before starting themselves.

The startup sequence, roughly:

1. **blockchain** starts first (no dependencies)
2. **dynamodb-local** and **redis** start (no dependencies)
3. **ipni** starts (no dependencies)
4. **signing-service** waits for blockchain
5. **delegator** waits for blockchain and dynamodb-local
6. **indexer** waits for redis and ipni
7. **piri** waits for blockchain, indexer, signing-service, delegator, and dynamodb-local
8. **upload** waits for indexer, dynamodb-local, and piri
9. **guppy** waits for upload and piri

### Piri's Multi-Step Initialization

Each piri node declared in `smelt.yml` (default 1, up to 9) runs its own multi-step initialization on first start:

1. **Extract DID**: Parse the node's `piri-{N}.pem` key to determine its `did:key` identity
2. **Register with allow list**: Add the DID to the delegator's DynamoDB allow list
3. **Register on-chain**: Register as a storage provider with the PDP contracts via signing-service
4. **Create proof set**: Submit a create-proof-set transaction and wait for confirmation
5. **Start server**: Begin accepting storage requests on port `3000` (mapped to host `4000 + N`)

All nodes initialize concurrently. First-time setup takes 1–3 minutes per node (with some amortization across parallel startup). Monitor a specific node with `docker compose logs -f piri-{N}`.

The entrypoint script (`systems/piri/entrypoint.sh`) is shared by every piri container; each container reads its own key, wallet, and DB/S3 config via environment variables injected by `generated/compose/piri.yml`.

### Expected Startup Time

| Phase | Duration |
|-------|----------|
| Image pull (first time) | 2-5 minutes |
| Key generation | 5-10 seconds |
| Service startup | 5-15 seconds |
| Piri registration (per node) | 20-40 seconds |
| **Total (first run)** | **3-8 minutes** |
| **Total (subsequent cold boot)** | **30-60 seconds** |
| **Total (snapshot-restored)** | **~10 seconds** |

See [SNAPSHOTS.md](SNAPSHOTS.md) for how to skip the registration cost
on subsequent boots.

---

## Verifying Health

After `make up` completes, check service status:

```bash
make status
```

This runs `docker compose ps` and highlights health states. A healthy network looks like:

```
NAME                     STATUS                        PORTS
smelt-blockchain-1       Up 1 minute (healthy)         0.0.0.0:8545->8545/tcp
smelt-delegator-1        Up 1 minute (healthy)         0.0.0.0:8081->80/tcp
smelt-dynamodb-local-1   Up 1 minute (healthy)         0.0.0.0:8000->8000/tcp
smelt-email-1            Up 1 minute
smelt-guppy-1            Up 1 minute
smelt-indexer-1          Up 1 minute (healthy)         0.0.0.0:9000->80/tcp
smelt-ipni-1             Up 1 minute (healthy)         0.0.0.0:3000-3003->3000-3003/tcp
smelt-ipni-init-1        Exited (0)
smelt-minio-1            Up 1 minute (healthy)         0.0.0.0:9010-9011->9000-9001/tcp
smelt-piri-0-1           Up 1 minute (healthy)         0.0.0.0:4000->3000/tcp
smelt-redis-1            Up 1 minute (healthy)         0.0.0.0:6379->6379/tcp
smelt-signing-service-1  Up 1 minute (healthy)         0.0.0.0:7446->7446/tcp
smelt-upload-1           Up 1 minute (healthy)         0.0.0.0:8080->80/tcp
```

`ipni-init` is a one-shot initializer that exits with code 0 after
setting up IPNI's data directory. Exited (0) is the correct final state.

### Understanding Health States

| State | Meaning |
|-------|---------|
| `healthy` | Service passed its health check |
| `starting` | Service is running but health check hasn't passed yet |
| `unhealthy` | Health check failed (check logs) |
| No health indicator | Service doesn't define a health check (guppy) |

### Services That Take Longer

**IPNI** (~30 seconds): The InterPlanetary Network Indexer needs time to initialize its datastore and start accepting queries.

**Piri** (~3 minutes): As described above, piri runs a multi-step initialization that registers with the blockchain and obtains delegations. The health check has a `start_period: 180s` to account for this.

**Indexer** (~30 seconds): Waits for IPNI to be healthy, then initializes its Redis connection and claim cache.

If services remain unhealthy after 5 minutes, something is wrong. Check logs.

---

## Your First Upload

Once all services are healthy, you can upload content. The guppy container provides a CLI for this.

### Enter the Guppy Shell

```bash
make shell-guppy
```

This opens a bash shell inside the guppy container. All subsequent commands in this section run inside this shell.

### Create an Identity

```bash
guppy login your@email.com
```

**What's happening**: Guppy generates an Ed25519 keypair and stores it in its local keystore (`/root/.storacha/guppy/`). The email is associated with this identity but isn't verified in local development—it's just a label.

### Create a Space

```bash
# Returns the space's did:key on stdout.
SPACE=$(guppy space generate)
echo "$SPACE"
```

**What's happening**: Guppy generates another Ed25519 keypair specifically
for this space. A space is a logical container for content — think of it
as a namespace with its own access controls. The DID looks like
`did:key:z6MkrZ...`.

### Add a Source and Upload

Guppy uploads are space-scoped and source-based: you first register one or
more files or directories as *sources* of a space, then `guppy upload
<SPACE>` ships every source's content.

```bash
# Create some test data (min 1 KiB; use randdir for something realistic)
echo "Hello Storacha" > /tmp/hello.txt

# Register the file as a source of the space
guppy upload source add "$SPACE" /tmp/hello.txt

# Upload every source in the space
guppy upload "$SPACE"
```

Output shows a content CID per source — save the CIDs for retrieval:

```
Upload completed successfully: bafybei...
```

**What's happening** (this is the interesting part):

1. **Sharding**: Guppy reads each source and creates content-addressed
   blocks. Small files become a single block; large files are split
   into multiple shards.
2. **UCAN invocations**: For each shard, guppy sends a `space/blob/add`
   invocation to the upload service.
3. **Blob allocation**: The upload service forwards a `blob/allocate`
   request to piri, which reserves space and returns a presigned
   upload URL.
4. **HTTP PUT**: Guppy uploads the bytes to piri.
5. **Blob acceptance**: Guppy signals completion via `ucan/conclude`;
   the upload service calls `blob/accept` on piri, which verifies the
   upload and emits a location claim.
6. **Claim caching**: The upload service pushes the location claim to
   the indexer via `claim/cache`, making content discoverable.
7. **Indexing**: Guppy sends `space/index/add` to register the content
   index, and `upload/add` to record the upload.

Subsequent `guppy upload "$SPACE"` runs behave like rsync — only changed
content is re-uploaded.

---

## Your First Retrieval

With content uploaded, retrieve it by CID via the space that owns it.

### Download the Content

```bash
# Still inside the guppy shell
guppy retrieve "$SPACE" bafybei... /tmp/retrieved.txt
```

Replace `bafybei...` with the actual CID from your upload.

**What's happening**:

1. **Query indexer**: Guppy contacts the indexer asking "where can I find content with this CID?" The indexer checks its cache and queries IPNI if needed.

2. **Location lookup**: The indexer returns location claims—signed assertions stating where the content can be retrieved (in this case, from piri).

3. **Authorized retrieval**: Guppy constructs a UCAN invocation with the `space/content/retrieve` capability and sends it to piri in the HTTP Authorization header.

4. **Content serving**: Piri verifies the UCAN delegation chain, confirms the requester has permission to access this content, and streams the bytes.

5. **Reassembly**: If the content was sharded, guppy fetches each shard and reassembles them.

### Verify the Content

```bash
cat /tmp/retrieved.txt
# Should output: Hello Storacha
```

---

## Exploring the Services

Now that you've completed an upload/retrieval cycle, here's how to inspect and debug the system.

### Viewing Logs

```bash
# All services (noisy but comprehensive)
make logs

# Specific service — use piri-0 (or piri-N) for piri services
docker compose logs -f piri-0

# Multiple services
docker compose logs -f upload indexer

# Last 100 lines only
docker compose logs --tail=100 piri-0
```

Log output varies by service. Piri and the indexer tend to be verbose;
the blockchain is quieter unless transactions are occurring.

### Shelling into Containers

```bash
# Guppy CLI
make shell-guppy

# Piri storage node (shells into piri-0 by default)
make shell-piri

# Any service (using docker compose directly)
docker compose exec indexer sh
docker compose exec delegator sh
docker compose exec blockchain sh

# Additional piri nodes: use the piri-N name
docker compose exec piri-1 sh
```

Most containers use Alpine Linux, so `sh` is available but `bash` may not be.

### Checking Service Endpoints

From your host machine (not inside containers):

**Blockchain (JSON-RPC)**
```bash
curl -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'
# Returns current block number
```

**Indexer (health check)**
```bash
curl http://localhost:9000/
# Returns empty response with 200 OK if healthy
```

**Piri (health check)**
```bash
curl http://localhost:4000/readyz
# Returns {"status":"ok"} if healthy. For additional nodes:
# curl http://localhost:4001/readyz, :4002/readyz, etc.
```

**Delegator (health check)**
```bash
curl http://localhost:8081/healthcheck
# Returns health status
```

**Signing Service (health check)**
```bash
curl http://localhost:7446/healthcheck
# Returns health status
```

**DynamoDB Local (web console)**

Open in your browser: `http://localhost:8000/shell/`

This provides a JavaScript shell for querying DynamoDB tables. Useful for inspecting:
- `delegator-allow-list`: DIDs authorized to receive delegations
- `delegator-provider-info`: Registered storage providers and their delegation proofs
- `upload-allocations`: Pending blob allocations
- `upload-receipts`: Stored UCAN receipts

**IPNI (finder)**
```bash
curl http://localhost:3000/health
# Returns "ready" if healthy
```

---

## Common First-Time Issues

### Services Unhealthy After 5 Minutes

First, check which service is unhealthy:

```bash
make status
```

Then check that service's logs:

```bash
docker compose logs --tail=200 <service-name>
```

Common causes:

- **IPNI unhealthy**: IPNI needs time to initialize. Wait another minute. If still failing, check for port conflicts on 3000-3003.

- **Piri unhealthy**: Piri's initialization is complex. Check logs for:
  - "Failed to extract DID" — key file issue
  - "Registration failed" — DynamoDB connectivity
  - "Init failed" — blockchain or delegator connectivity

- **Indexer unhealthy**: Usually means Redis isn't ready. Check Redis health first.

- **Upload unhealthy**: Check that piri and indexer are healthy first (upload depends on both).

### No Space to Run Docker

```
Error response from daemon: no space left on device
```

Docker images and volumes consume disk space. To reclaim:

```bash
# Remove unused images, containers, and volumes
docker system prune -a --volumes

# Warning: This removes ALL Docker data, not just Smelt's
```

Or free up space on your disk and try again.

### mkdelegation Not Found

If you see warnings about missing `mkdelegation` during `make init`:

```bash
# Option 1: Install Go and re-run init
brew install go  # or your package manager
make init

# Option 2: Install mkdelegation directly
go install github.com/storacha/go-mkdelegation@latest

# Option 3: Manual install to PATH
GOBIN=/usr/local/bin go install github.com/storacha/go-mkdelegation@latest
```

The tool must be in your PATH for the init script to find it.

### Permission Denied on generated/

```
permission denied: generated/keys/piri.pem
```

This usually happens if you previously ran Docker as root or with different permissions:

```bash
sudo chown -R $USER:$USER generated/
```

### Port Already in Use

```
Error starting userland proxy: listen tcp4 0.0.0.0:8545: bind: address already in use
```

Something else is using that port. Find it:

```bash
# macOS/Linux
lsof -i :8545
# or
netstat -tlnp | grep 8545
```

Either stop the conflicting process or modify the port mappings in the relevant `compose.yml` file.

### Guppy Commands Fail with UCAN Errors

```
Error: UCAN validation failed: audience mismatch
```

This usually means the guppy config doesn't match the running services. Check that `systems/guppy/config/guppy-config.toml` has the correct DIDs:

```toml
upload_id = "did:web:upload"
upload_url = "http://upload:80"
indexer_id = "did:web:indexer"
indexer_url = "http://indexer:80"
```

If you regenerated keys, you may need to restart guppy:

```bash
docker compose restart guppy
```

### "Handler Not Found" Errors

```
Error: handler not found for capability: space/blob/add
```

The upload service may not be running the expected version. Rebuild:

```bash
make down
docker compose build upload
make up
```

---

## Stopping and Cleaning Up

Smelt provides several levels of cleanup, from gentle to nuclear.

### Stop Services, Keep Data

```bash
make down
```

Stops all containers but preserves Docker volumes. On graceful shutdown
the blockchain container dumps the current anvil state to
`generated/snapshot-scratch/`, so your uploaded content, contract state,
and service state all persist. Next `make up` resumes from exactly where
you left off.

### Stop Services, Delete Volumes

```bash
make clean
```

Stops containers, deletes Docker volumes, and resets the scratch chain
state plus any active snapshot session. Keys and proofs in `generated/`
are preserved. The next `make up` cold-boots from the committed baseline
with the same service identities.

### Delete Everything

```bash
make nuke
```

Removes containers, volumes, keys, proofs, locally-built Docker images,
and scratch state. Complete reset. The next `make up` regenerates keys
(new DIDs), pulls images, and initializes from scratch.

### Fresh Start

```bash
make fresh
```

Equivalent to `make nuke` followed by `make init`, `docker compose build`,
and `make up`. One command to destroy everything and rebuild.

Both `make clean` and `make nuke` prompt for confirmation. Skip the
prompt with:

```bash
make nuke YES=1
```

### Skip the cold boot next time

Once a stack is healthy, `./smelt snapshot save baseline` captures it.
Later boots via `make up SNAPSHOT=baseline` reach the same state in
~10s instead of ~45s by skipping contract deploy and piri registration.
See [SNAPSHOTS.md](SNAPSHOTS.md) for the full story.

---

## Next Steps

You've successfully set up Smelt and completed your first upload/retrieval cycle. Here's where to go from here:

### Understand the Architecture

Read [docs/ARCHITECTURE.md](ARCHITECTURE.md) for a detailed explanation of:
- How services communicate
- The UCAN capability system
- Content claims and indexing
- The complete upload and retrieval flows

### Experiment with Larger Files

Upload files larger than 1MB to see sharding in action:

```bash
# Inside guppy shell
dd if=/dev/urandom of=/tmp/large.bin bs=1M count=10
guppy upload --replicas=1 /tmp/large.bin
```

Watch the logs to see multiple `space/blob/add` invocations as guppy shards the file.

### Inspect the Blockchain

The blockchain runs Anvil with pre-deployed PDP (Provable Data Possession) smart contracts. You can interact with it using any Ethereum tooling:

```bash
# Using cast (from Foundry)
cast block-number --rpc-url http://localhost:8545
cast balance 0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266 --rpc-url http://localhost:8545
```

### Modify Guppy's Configuration

The guppy config at `systems/guppy/config/guppy-config.toml` controls which services guppy connects to. You can point it at different indexers or upload services for testing.

### Read the Specifications

For deep protocol understanding:
- [W3 Blob Protocol](https://github.com/storacha/specs/blob/main/w3-blob.md)
- [W3 Index Protocol](https://github.com/storacha/specs/blob/main/w3-index.md)
- [W3 Retrieval Protocol](https://github.com/storacha/specs/blob/main/w3-retrieval.md)

---

## Quick Reference

| Task | Command |
|------|---------|
| Start network | `make up` |
| Stop network | `make down` |
| View status | `make status` |
| View logs | `make logs` |
| Guppy shell | `make shell-guppy` |
| Piri shell | `make shell-piri` |
| Full reset | `make fresh` |
| Help | `make help` |

| Service | Host Port | Health Check |
|---------|-----------|--------------|
| Blockchain | 8545 | `curl -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'` |
| DynamoDB | 8000 | `http://localhost:8000/shell/` (browser) |
| Redis | 6379 | `redis-cli -h localhost ping` |
| Signing Service | 7446 | `curl localhost:7446/healthcheck` |
| Delegator | 8081 | `curl localhost:8081/healthcheck` |
| IPNI | 3000 | `curl localhost:3000/health` |
| Indexer | 9000 | `curl localhost:9000/` |
| Piri-0 | 4000 | `curl localhost:4000/readyz` |
| Piri-N | 4000+N | `curl localhost:$((4000+N))/readyz` |
| Upload | 8080 | `curl localhost:8080/health` |
