# Signing Service System

PDP (Proof of Data Possession) operation signing for storage providers.

## Services

- **signing-service** - Signs blockchain transactions for PDP proofs

## Ports

| Port | Service | Description |
|------|---------|-------------|
| 7446 | signing-service | Signing API |

## Configuration

- `config/signer.yaml` - Signing service configuration

## Keys

- `../../generated/keys/payer-key.hex` - EVM private key for signing

## Standalone Usage

```bash
# Requires blockchain to be running
cd systems/signing-service
docker compose up -d
```

## Dependencies

- blockchain (service_healthy)

## Used By

- piri
