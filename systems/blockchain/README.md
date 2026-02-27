# Blockchain System

Local Filecoin blockchain using Anvil for smart contract interactions.

## Services

- **blockchain** - Anvil-based local Filecoin chain with pre-deployed contracts

## Ports

| Port | Service | Description |
|------|---------|-------------|
| 8545 | blockchain | JSON-RPC endpoint |

## Configuration

No configuration files. Uses environment variables:
- `ANVIL_BLOCK_TIME=3` - Block time in seconds

## Standalone Usage

```bash
cd systems/blockchain
docker compose up -d
```

## Dependencies

None - this is a foundational service.

## Used By

- signing-service
- delegator
- piri
