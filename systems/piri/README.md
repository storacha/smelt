# Piri System

Storage node with PDP (Proof of Data Possession) proofs.

## Services

- **piri** - Storage provider that stores and serves content
- **piri-postgres** (optional) - PostgreSQL database backend
- **piri-minio** (optional) - S3-compatible blob storage

## Ports

| Port | Service | Description |
|------|---------|-------------|
| 4000 | piri | Piri API (container port 3000) |
| 9002 | piri-minio | MinIO S3 API (when using S3 backend) |
| 9003 | piri-minio | MinIO Console (when using S3 backend) |

## Configuration

- `config/piri-base-config.toml` - Base configuration (contracts, services)
- `config/piri-db-postgres.toml` - PostgreSQL database configuration
- `config/piri-blob-s3.toml` - S3 blob storage configuration
- `config/piri-overrides.toml` - Additional overrides (telemetry, etc.)
- `entrypoint.sh` - Initialization and startup script

## Keys

- `../../generated/keys/piri.pem` - Ed25519 identity key

## Volumes

- `piri-data` - Storage node persistent data

## Storage Backend Configuration

Piri supports two independent storage axes that can be combined:

### Database Backend
- **sqlite** (default) - Local SQLite database
- **postgres** - PostgreSQL database

### Blob Storage Backend
- **filesystem** (default) - Local filesystem storage
- **s3** - S3-compatible storage (MinIO)

## Standalone Usage

```bash
# Default: SQLite + Filesystem
docker compose up -d

# PostgreSQL + Filesystem
docker compose --profile piri-postgres up -d

# SQLite + S3 (MinIO)
docker compose --profile piri-s3 up -d

# PostgreSQL + S3 (MinIO)
docker compose --profile piri-postgres --profile piri-s3 up -d
```

## Initialization

Piri initialization uses the selected storage backends from the start:

1. Storage backend configs are merged into base-config BEFORE init
2. `piri init` registers with blockchain using the configured backends
3. State is stored in PostgreSQL/S3 during initialization (not after)
4. `piri serve full` uses the same backends with consistent state

The `entrypoint.sh` script handles this automatically based on:
- `PIRI_DB_BACKEND` - Database backend ("sqlite" or "postgres")
- `PIRI_BLOB_BACKEND` - Blob storage backend ("filesystem" or "s3")

## Dependencies

- blockchain (service_healthy)
- indexer (service_healthy)
- signing-service (service_healthy)
- delegator (service_healthy)
- dynamodb-local (service_healthy)

## Used By

- upload
- guppy
