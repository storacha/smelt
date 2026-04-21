# Common System

Shared infrastructure services used by multiple systems.

## Services

- **dynamodb-local** - Local DynamoDB for state persistence
- **minio** - Local S3-compatible storage
- **smtp4dev** - Local SMTP server with Web UI and REST API

## Ports

| Host Port | Container Port | Service | Description |
|-----------|----------------|---------|-------------|
| 15010 | 8000 | dynamodb-local | DynamoDB endpoint |
| 15070 | 9000 | minio | S3 API endpoint |
| 15071 | 9001 | minio | Console endpoint |
| 15080 | 25   | email (smtp4dev) | SMTP endpoint |
| 15081 | 80   | email (smtp4dev) | Web UI / API |

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
