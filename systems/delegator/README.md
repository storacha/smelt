# Delegator System

UCAN delegation service (registrar) for storage providers.

## Services

- **delegator** - Issues UCAN delegations to storage providers

## Ports

| Port | Service | Description |
|------|---------|-------------|
| 8081 | delegator | Delegator API (container port 8080) |

## Configuration

- `config/delegator.yaml` - Delegator service configuration

## Keys

- `../../generated/keys/delegator.pem` - Ed25519 identity key

## Standalone Usage

```bash
# Requires blockchain and dynamodb-local to be running
cd systems/delegator
docker compose up -d
```

## Dependencies

- blockchain (service_healthy)
- dynamodb-local (service_healthy)

## Used By

- piri
