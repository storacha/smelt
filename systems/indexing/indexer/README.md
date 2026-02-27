# Indexer Sub-Package

Storacha's content claims caching layer.

## Services

- **redis** - Cache backend for the indexing service
- **indexer** - Content claims caching and query service

## Ports

| Port | Service | Description |
|------|---------|-------------|
| 6379 | redis | Redis cache |
| 9000 | indexer | Indexing service API (container port 80) |

## Configuration

Configuration via environment variables and command-line flags.

## Keys

- `../../../generated/keys/indexer.pem` - Ed25519 identity key

## Standalone Usage

```bash
# Requires IPNI to be running first
cd systems/indexing/indexer
docker compose up -d
```

## Dependencies

- ipni (service_healthy) - for content discovery
- redis (service_healthy) - internal dependency

## Used By

- piri
- upload
- guppy
