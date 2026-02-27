# Common System

Shared infrastructure services used by multiple systems.

## Services

- **dynamodb-local** - Local DynamoDB for state persistence

## Ports

| Port | Service | Description |
|------|---------|-------------|
| 8000 | dynamodb-local | DynamoDB endpoint |

## Standalone Usage

```bash
cd systems/common
docker compose up -d
```

## Dependencies

None - this is a foundational service.

## Used By

- delegator (provider info, allow list)
- upload (allocations, receipts, auth requests, provisionings, uploads)
- piri (allow list registration)
