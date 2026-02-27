# Piri System

Storage node with PDP (Proof of Data Possession) proofs.

## Services

- **piri** - Storage provider that stores and serves content

## Ports

| Port | Service | Description |
|------|---------|-------------|
| 3333 | piri | Piri API (container port 3000) |

## Configuration

- `config/piri-config.toml` - Main Piri configuration
- `config/piri-config-docker.toml` - Docker-specific configuration
- `entrypoint.sh` - Initialization and startup script

## Keys

- `../../generated/keys/piri.pem` - Ed25519 identity key

## Volumes

- `piri-data` - Storage node persistent data

## Initialization

Piri uses a two-step initialization process:
1. `piri init` - Registers with blockchain and delegator, generates config
2. `piri serve` - Starts server using generated config

The `entrypoint.sh` script handles this automatically.

## Standalone Usage

```bash
# Requires blockchain, signing-service, delegator, indexer, dynamodb-local
cd systems/piri
docker compose up -d
```

## Dependencies

- blockchain (service_healthy)
- indexer (service_healthy)
- signing-service (service_healthy)
- delegator (service_healthy)
- dynamodb-local (service_healthy)

## Used By

- upload
- guppy
