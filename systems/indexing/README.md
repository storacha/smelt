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

| Host Port | Container Port | Service | Description |
|-----------|----------------|---------|-------------|
| 15020 | 6379 | redis | Redis cache |
| 15050 | 80   | indexer | Indexing service |
| 15090 | 3000 | ipni | IPNI finder (queries) |
| 15091 | 3002 | ipni | IPNI admin |
| 15092 | 3003 | ipni | IPNI p2p (advertisement sync) |

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
