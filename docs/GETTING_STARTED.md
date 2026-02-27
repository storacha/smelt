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

### Docker and Docker Compose

Docker Compose V2 is required (the `docker compose` subcommand, not the legacy `docker-compose` binary). Most Docker Desktop installations from 2022 onward include this by default.

```bash
# Verify your installation
docker --version          # Any recent version works
docker compose version    # Should show "Docker Compose version v2.x.x"
```

If `docker compose` fails but `docker-compose` works, you have the legacy version. Upgrade Docker Desktop or install the compose plugin separately.

### Go 1.22+

Go is required for generating UCAN delegation proofs. The setup process installs the `mkdelegation` tool, which creates the cryptographic proofs that allow services to authorize each other.

```bash
go version    # Should show "go1.22" or higher
```

If Go is unavailable, the setup will warn you and skip proof generation. The network will start, but certain service-to-service authorization flows may fail.

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
- **Windows**: WSL2 only (native Windows Docker has networking issues with this configuration)

---

## Installation

Clone the repository and enter the directory:

```bash
git clone https://github.com/storacha/smelt.git
cd smelt
```

That's it. The actual initialization happens when you first start the network.

---

## Understanding the Setup Process

When you run `make up` for the first time (or `make init` explicitly), the system prepares the environment through several distinct phases. Understanding these phases helps when something goes wrong—and something always goes wrong eventually.

### What `make init` Does

The initialization script (`scripts/init.sh`) performs five steps:

#### Step 1: Create Directory Structure

```
generated/
  keys/       # Cryptographic keys for service identities
  proofs/     # UCAN delegation proofs for service authorization
```

These directories are gitignored. Your keys are local to your machine.

#### Step 2: Generate Ed25519 Keypairs

The script generates PEM-format Ed25519 keys for each service that needs a cryptographic identity:

| Key File | Service | Purpose |
|----------|---------|---------|
| `piri.pem` | Piri storage node | Signs storage commitments and content claims |
| `upload.pem` | Upload service | Signs upload coordination messages |
| `indexer.pem` | Indexer | Signs index claims |
| `delegator.pem` | Delegator | Issues capability delegations |
| `signing-service.pem` | Signing service | Signs PDP blockchain operations |
| `etracker.pem` | Egress tracker | Signs egress tracking claims |

Each key generates a corresponding `did:key` identifier. For example, the piri key might produce `did:key:z6MkfYoQ6dppqssZ9qHF6PbBzCjoS1wWg15GYxNaMiLZn5RD`. These identifiers appear throughout logs and error messages.

#### Step 3: Extract EVM Keys from Blockchain State

The local blockchain (Anvil) ships with pre-funded accounts. The setup extracts two keys:

| Key File | Source | Purpose |
|----------|--------|---------|
| `payer-key.hex` | `.payer.privateKey` | Pays gas fees for PDP operations |
| `owner-wallet.hex` | `.deployer.privateKey` | Registers piri as a storage provider |

These keys are extracted from `systems/blockchain/state/deployed-addresses.json`, which contains the addresses and keys used when the smart contracts were originally deployed.

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

### Piri's Two-Phase Initialization

Piri requires special attention. Unlike other services that simply start, piri runs a multi-step initialization:

1. **Extract DID**: Parse the Ed25519 key to determine piri's `did:key` identity
2. **Register with allow list**: Add the DID to the delegator's DynamoDB allow list
3. **Initialize piri**: Register with the blockchain, obtain delegations from the delegator
4. **Start server**: Begin accepting storage requests

This initialization takes 1-3 minutes on first run. The entrypoint script (`systems/piri/entrypoint.sh`) orchestrates this process.

### Expected Startup Time

| Phase | Duration |
|-------|----------|
| Image pull (first time) | 2-5 minutes |
| Key generation | 5-10 seconds |
| Service startup | 30-60 seconds |
| Piri initialization | 1-3 minutes |
| **Total (first run)** | **3-8 minutes** |
| **Total (subsequent)** | **1-3 minutes** |

---

## Verifying Health

After `make up` completes, check service status:

```bash
make status
```

This runs `docker compose ps` and highlights health states. A healthy network looks like:

```
NAME                    STATUS                   PORTS
blockchain              Up 2 minutes (healthy)   0.0.0.0:8545->8545/tcp
delegator               Up 2 minutes (healthy)   0.0.0.0:8081->80/tcp
dynamodb-local          Up 2 minutes (healthy)   0.0.0.0:8000->8000/tcp
guppy                   Up About a minute
indexer                 Up 2 minutes (healthy)   0.0.0.0:9000->80/tcp
ipni                    Up 2 minutes (healthy)   0.0.0.0:3000-3003->3000-3003/tcp
piri                    Up 2 minutes (healthy)   0.0.0.0:3333->3000/tcp
redis                   Up 2 minutes (healthy)   0.0.0.0:6379->6379/tcp
signing-service         Up 2 minutes (healthy)   0.0.0.0:7446->7446/tcp
upload                  Up About a minute (healthy)   0.0.0.0:8080->80/tcp
```

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
guppy space create my-space
```

**What's happening**: Guppy generates another Ed25519 keypair specifically for this space. A space is a logical container for content—think of it as a namespace with its own access controls. The space gets a `did:key` identifier like `did:key:z6MkrZ...`.

Guppy automatically selects the newly created space as the current space.

### Upload a File

```bash
echo "Hello Storacha" > /tmp/hello.txt
guppy upload /tmp/hello.txt
```

**What's happening** (this is the interesting part):

1. **Sharding**: Guppy reads the file and creates content-addressed blocks. Small files become a single block; large files are split into multiple shards.

2. **UCAN invocations**: For each shard, guppy sends a `space/blob/add` invocation to the upload service, requesting storage allocation.

3. **Blob allocation**: The upload service forwards a `blob/allocate` request to piri, which reserves space and returns a presigned upload URL.

4. **HTTP PUT**: Guppy uploads the actual bytes to piri via HTTP PUT to the presigned URL.

5. **Blob acceptance**: Guppy signals completion via `ucan/conclude`. The upload service calls `blob/accept` on piri, which verifies the upload and generates a location claim.

6. **Claim caching**: The upload service sends the location claim to the indexer via `claim/cache`, making the content discoverable.

7. **Indexing**: Guppy sends `space/index/add` to register the content index, and `upload/add` to record the upload.

The output shows a content CID—the content-addressed identifier for your uploaded file. Save this for retrieval.

```
Uploaded: /tmp/hello.txt
Root CID: bafybei...
```

---

## Your First Retrieval

With content uploaded, you can retrieve it using the CID from the upload output.

### Download the Content

```bash
# Still inside the guppy shell
guppy download bafybei... /tmp/retrieved.txt
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

# Specific service
docker compose logs -f piri

# Multiple services
docker compose logs -f upload indexer

# Last 100 lines only
docker compose logs --tail=100 piri
```

Log output varies by service. Piri and the indexer tend to be verbose; the blockchain is quieter unless transactions are occurring.

### Shelling into Containers

```bash
# Guppy CLI
make shell-guppy

# Piri storage node
make shell-piri

# Any service (using docker compose directly)
docker compose exec indexer sh
docker compose exec delegator sh
docker compose exec blockchain sh
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
curl http://localhost:3333/
# Returns empty response with 200 OK if healthy
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

Stops all containers but preserves Docker volumes. Your uploaded content, blockchain state, and configuration remain intact. Next `make up` restarts quickly with existing data.

### Stop Services, Delete Volumes

```bash
make clean
```

Stops containers and deletes Docker volumes. This removes:
- Uploaded content in piri
- IPNI index data
- Redis cache
- DynamoDB tables

Keys and proofs in `generated/` are preserved. The network will reinitialize on next start, but with the same identities.

### Delete Everything

```bash
make nuke
```

Removes containers, volumes, keys, proofs, and locally-built Docker images. Complete reset. The next `make up` will:
- Generate new keys (new DIDs for all services)
- Pull images fresh
- Initialize from scratch

### Fresh Start

```bash
make fresh
```

Equivalent to `make nuke` followed by `make init`, `docker compose build`, and `make up`. One command to destroy everything and rebuild.

Both `make clean` and `make nuke` prompt for confirmation. Skip the prompt with:

```bash
make nuke YES=1
```

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
guppy upload /tmp/large.bin
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
| Piri | 3333 | `curl localhost:3333/` |
| Upload | 8080 | `curl localhost:8080/health` |
