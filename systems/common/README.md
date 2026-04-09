# Common System

Shared infrastructure services used by multiple systems.

## Services

- **dynamodb-local** - Local DynamoDB for state persistence
- **minio** - Local S3-compatible storage
- **smtp4dev** - Local SMTP server with Web UI and REST API

## Ports

| Port | Service | Description |
|------|---------|-------------|
| 2525 | email   | SMTP endpoint |
| 2580 | email   | Web UI / API |
| 8000 | dynamodb-local | DynamoDB endpoint |
| 9002 | minio | S3 API endpoint |
| 9003 | minio | Console endpoint |

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
