# Piri System

Storage node(s) with PDP (Proof of Data Possession) proofs.

> **Note**: This directory is not a standalone compose stack. Piri services are generated from `smelt.yml` by `cmd/smelt/` and written to `generated/compose/piri.yml`. Do not run `docker compose up` from this directory — it will not produce a working setup. See [../../docs/MULTI_PIRI.md](../../docs/MULTI_PIRI.md) for how to configure and run piri nodes, and [../../docs/GETTING_STARTED.md](../../docs/GETTING_STARTED.md) for first-time setup.

## What the generator emits

One or more `piri-{N}` services, each configured according to the corresponding entry in `smelt.yml`. Shared `piri-postgres` (with a `piri-postgres-init` sidecar) and/or `piri-minio` services are included when any node selects those backends.

## Ports

| Host Port | Service | Description |
|-----------|---------|-------------|
| 4000 + N  | piri-{N} | Piri API, one container per node (container port 3000) |
| 5432      | piri-postgres | Shared postgres instance (only when any node uses `db: postgres`) |
| 9002      | piri-minio | Shared MinIO S3 API (only when any node uses `blob: s3`) |
| 9003      | piri-minio | MinIO console |

## Files in this directory

- `config/piri-base-config.toml` — Base configuration (contract addresses, service DIDs). Mounted read-only into every piri container.
- `config/piri-overrides.toml` — Additional overrides merged after init.
- `entrypoint.sh` — Shared startup script mounted into every piri container. Reads environment variables injected by the generator (`PIRI_DB_BACKEND`, `PIRI_BLOB_BACKEND`, `PIRI_DB_POSTGRES_URL`, `PIRI_S3_*`, etc.) to decide which backends to use.
- `register-did.sh` — Helper script for DynamoDB allow-list registration during init.

## Keys (generated)

- `../../generated/keys/piri-{N}.pem` — Ed25519 identity, one per node
- `../../generated/keys/piri-{N}-wallet.hex` — EVM wallet derived from an Anvil pre-funded account (account 0 for piri-0; account N + 1 for piri-N with N ≥ 1)

## Volumes

- `piri-{N}-data` — Per-node persistent data (one volume per node)

## Dependencies

- blockchain (service_healthy)
- indexer (service_healthy)
- signing-service (service_healthy)
- delegator (service_healthy)
- dynamodb-local (service_healthy)

## Used By

- upload (via service DID resolution at request time; no static `depends_on`)
- guppy (via upload, and directly for blob retrieval)
