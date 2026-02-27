# Upload System

Mock w3infra replacement for upload coordination.

## Services

- **upload** - Handles upload coordination, blob allocation, and UCAN processing

## Ports

| Port | Service | Description        |
|------|---------|--------------------|
| 8080 | upload  | Upload service API |

## Configuration

Configuration via environment variables (see compose.yml).

## Keys

Currently uses `PRIVATE_KEY` environment variable (base64-encoded).
TODO: Switch to PEM file loading.

## Standalone Usage

```bash
# Requires indexer, dynamodb-local, piri to be running
cd systems/upload
docker compose up -d
```

## Dependencies

- indexer (service_healthy)
- dynamodb-local (service_healthy)
- piri (service_healthy)

## Used By

- guppy
