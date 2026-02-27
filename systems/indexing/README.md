# Indexing System

Content discovery and claims caching layer.

## Sub-Packages

This system is composed of two sub-packages:

### ipni/
InterPlanetary Network Indexer for content discovery.
- **ipni-init** - One-time configuration initialization
- **ipni** - Content discovery indexer (storetheindex)

### indexer/
Storacha's content claims caching layer.
- **redis** - Cache backend
- **indexer** - Content claims caching and query service

## Ports

| Port | Service | Description |
|------|---------|-------------|
| 3000 | ipni | IPNI finder (queries) |
| 3002 | ipni | IPNI admin |
| 3003 | ipni | IPNI ingest (advertisement sync) |
| 6379 | redis | Redis cache |
| 9000 | indexer | Indexing service (container port 80) |

## Standalone Usage

Start the entire indexing system:
```bash
cd systems/indexing
docker compose up -d
```

Or start sub-packages individually:
```bash
# Start IPNI first
cd systems/indexing/ipni
docker compose up -d

# Then start indexer (depends on IPNI)
cd systems/indexing/indexer
docker compose up -d
```

## Dependencies

Internal dependencies:
- indexer depends on ipni, redis

## Used By

- piri
- upload
- guppy
