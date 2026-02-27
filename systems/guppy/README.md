# Guppy System

CLI client for uploading and retrieving content.

## Services

- **guppy** - Interactive CLI for uploads and retrievals

## Configuration

- `config/guppy-config.toml` - Client configuration

## Volumes

- `guppy-data` - Client state persistence
- `../../uploads` - Upload/download staging directory

## Usage

```bash
# Access the guppy shell
make guppy
# Or directly:
docker compose exec guppy bash

# Inside the container:
guppy upload /uploads/myfile.txt
guppy retrieve <cid> -o /uploads/retrieved.txt
```

## Standalone Usage

```bash
# Requires upload, piri to be running
cd systems/guppy
docker compose up -d
```

## Dependencies

- upload (service_healthy)
- piri (service_healthy)
