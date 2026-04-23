# Upload System

Mock w3infra replacement for upload coordination.

## Services

- **upload** - Handles upload coordination, blob allocation, and UCAN processing
- **postgres** - Sprue's metadata store; isolated to this system (separate from any piri-postgres)

## Ports

| Host Port | Container Port | Service | Description |
|-----------|----------------|---------|-------------|
| 15060 | 80 | upload | Upload service API |
| 15061 | 5432 | postgres | Sprue metadata store (psql from host, debugging) |

## Configuration

Configuration via environment variables (see compose.yml).

## Keys

Currently uses `PRIVATE_KEY` environment variable (base64-encoded).
TODO: Switch to PEM file loading.

## Standalone Usage

```bash
# Requires indexer, minio, piri to be running
cd systems/upload
docker compose up -d
```

## Dependencies

- indexer (service_healthy)
- minio (service_healthy)
- postgres (service_healthy)
- piri (service_healthy)

## Used By

- guppy
