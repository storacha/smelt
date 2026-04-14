# Multi-Piri Support

Smelt supports running N piri storage nodes from a single declarative manifest. This enables testing multi-provider scenarios: data replication, provider failover, concurrent uploads across storage providers, and mixed storage backend configurations.

## Objective

The Storacha network is designed around multiple storage providers. Testing with a single piri node leaves important behaviors unverified — how the upload service routes to multiple providers, how indexing works when content is spread across nodes, and how the system behaves under provider heterogeneity (different storage backends, different versions).

Multi-piri support lets developers:

- Spin up N piri nodes with one config change
- Mix storage backends per node (sqlite vs postgres, filesystem vs S3)
- Hot-add and hot-remove nodes without restarting the stack
- Test with different piri images per node (e.g., upgrade scenarios)

## How It Works

### The Manifest (`smelt.yml`)

A YAML file at the project root declares the desired piri topology:

```yaml
version: 1
piri:
  defaults:
    storage:
      db: sqlite
      blob: filesystem
  nodes:
    - name: piri-0
    - name: piri-1
      storage:
        db: postgres
        blob: s3
    - name: piri-2
      storage:
        db: postgres
```

Alternatively, use the `count` shorthand for identical nodes:

```yaml
version: 1
piri:
  count: 3
  defaults:
    storage:
      db: postgres
      blob: s3
```

Node names are always auto-generated as `piri-0`, `piri-1`, etc. Each node inherits from `defaults` unless overridden. If neither `count` nor `nodes` is specified, a single `piri-0` node is created.

### Generation Pipeline

Running `make generate` (or implicitly via `make up`) invokes the Go CLI tool:

```
smelt.yml
    │
    ▼
go run ./cmd/smelt generate
    │
    ├── generated/compose/piri.yml        Docker Compose services for N piri nodes
    └── generated/keys/
        ├── piri-0.pem                    Ed25519 identity key
        ├── piri-0-wallet.hex             EVM wallet (piri format)
        ├── piri-1.pem
        ├── piri-1-wallet.hex
        ├── upload.pem                    Non-piri service keys (unchanged)
        ├── indexer.pem
        └── ...
```

The root `compose.yml` includes the generated files:

```yaml
include:
  # ... static systems ...
  - path: generated/compose/piri.yml
  # ... static systems ...
```

### Per-Node Isolation

Each piri node gets:

| Resource | Pattern | Example |
|----------|---------|---------|
| Service name | `piri-{i}` | `piri-0`, `piri-1` |
| Host port | `4000 + i` | `4000`, `4001`, `4002` |
| Identity key | `piri-{i}.pem` | Unique DID per node |
| EVM wallet | `piri-{i}-wallet.hex` | From Anvil pre-funded accounts |
| Data volume | `piri-{i}-data` | Isolated persistent storage |
| PUBLIC_URL | `http://piri-{i}:3000` | Docker DNS resolution |

Keys and wallets are mounted into each container at the standard paths (`/keys/piri.pem`, `/keys/owner-wallet.hex`) so the entrypoint script requires no changes — it sees the right key at the expected path regardless of which node it is.

### Shared Infrastructure

Nodes that use postgres or S3 share a single instance of each, namespaced per node:

**Postgres** — One `piri-postgres` instance. Each postgres-backed node gets its own database (`piri_0`, `piri_1`, etc.). A `piri-postgres-init` sidecar creates databases idempotently on every startup, supporting hot-add without wiping the postgres volume.

**MinIO** — One `piri-minio` instance. Each S3-backed node uses a unique bucket prefix (`piri-0-`, `piri-1-`, etc.) via the `PIRI_S3_BUCKET_PREFIX` environment variable.

Shared infra services are only included in the generated compose when at least one node uses that backend. If all nodes use sqlite/filesystem, no postgres or minio services are created.

### Wallet Provisioning

Each piri node requires its own funded EVM wallet for on-chain registration. Wallets are derived from Anvil's 10 deterministic pre-funded accounts:

| Piri Node | Anvil Account |
|-----------|--------------|
| piri-0 | Account 0 (deployer) |
| piri-1 | Account 2 |
| piri-2 | Account 3 |
| piri-N | Account N+1 |

Account 1 is reserved for the signing-service payer. This limits the maximum to **9 piri nodes**.

### Hot-Add and Hot-Remove

**Adding a node:** Edit `smelt.yml` to add a node, then run `make up`. The generator produces updated compose files and creates keys for the new node (existing keys are not regenerated). Docker Compose starts the new service while leaving existing ones running.

**Removing a node:** Edit `smelt.yml` to remove a node, then run `make up`. Docker Compose's `--remove-orphans` flag detects that the removed service is no longer in the compose files and stops its container. The data volume is preserved (use `make clean` to remove volumes).

## Design

### Package Structure

```
cmd/smelt/              CLI tool (cobra)
  cmd/
    root.go             Root command
    generate.go         `smelt generate` subcommand

pkg/manifest/           Manifest schema and resolution
  manifest.go           Types: Manifest, PiriSpec, ResolvedPiriNode
  parse.go              YAML parsing

pkg/generate/           Generation logic (shared by CLI and pkg/stack)
  generate.go           Orchestration: parse → resolve → generate keys + compose
  compose.go            Docker Compose YAML generation for N piris
  keys.go               Ed25519 + EVM wallet generation
  anvil.go              Hardcoded Anvil account constants
```

### Key Design Decisions

**Manifest replaces profiles.** The old `--profile piri-postgres` / `--profile piri-s3` approach cannot express per-node storage configurations. The manifest declares storage backends per node, and the generator conditionally includes shared infra services. The Makefile targets `up-piri-postgres`, `up-piri-s3`, and `up-piri-postgres-s3` have been removed.

**Generator in Go.** The generation logic lives in `pkg/generate/` so it can be called both from the CLI (`cmd/smelt/`) and from the Go test stack (`pkg/stack/`). This avoids maintaining parallel shell and Go implementations.

**Volume-mount key isolation.** Rather than changing the piri entrypoint to accept a configurable key path, each container mounts its specific key file (`piri-{i}.pem`) to the standard container path (`/keys/piri.pem`). This requires zero changes to the entrypoint script.

**Idempotent key generation.** The generator skips existing key files unless `--force` is passed. This ensures hot-add creates keys for new nodes without regenerating keys for existing ones (which would invalidate their on-chain registration).

**Shared postgres with init sidecar.** Instead of relying on `docker-entrypoint-initdb.d/` (which only runs on first postgres start), a `piri-postgres-init` sidecar runs idempotent `CREATE DATABASE` commands on every startup. This correctly handles hot-add scenarios where a new database is needed in an already-initialized postgres instance.

### Go Test Stack Integration

The `pkg/stack/` package supports multi-piri via new options:

```go
// 3 identical nodes with default storage
s := stack.MustNewStack(t, stack.WithPiriCount(3))

// Heterogeneous nodes
s := stack.MustNewStack(t, stack.WithPiriNodes(
    stack.PiriNodeConfig{Postgres: true, S3: true},
    stack.PiriNodeConfig{},
))

// Access endpoints
s.PiriEndpoint()     // piri-0 (backward compat)
s.PiriEndpointN(1)   // piri-1
s.PiriCount()        // number of nodes
```

The existing `WithPiriPostgres()` and `WithPiriS3()` options continue to work for the single-node case via backward-compatible resolution.

## Limitations

- Maximum 9 piri nodes (constrained by Anvil's 10 pre-funded accounts, minus 1 for the signing-service payer).
- The indexer's `RESOLVE_DID_WEB` environment variable still references `did:web:piri` (singular). This does not currently break functionality but may need updating for full multi-provider DID resolution.
- Hot-remove does not automatically clean up data volumes. Use `make clean` or `docker volume rm` manually.

## Quick Reference

```bash
# Edit manifest
vim smelt.yml

# Apply changes (generates compose + keys, starts/stops services)
make up

# Generate without starting
make generate

# Force-regenerate all keys
go run ./cmd/smelt generate --force

# Shell into a specific piri node
docker compose exec piri-1 sh
```
