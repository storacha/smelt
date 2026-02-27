# Extending Smelt

This guide covers how to customize and extend Smelt for your particular development needs. The environment is designed to be modified—that's rather the point of having it run locally.

## Adding a New Service

Every service in Smelt lives in its own directory under `systems/`. This keeps concerns separated and makes it possible to run services individually when the full stack feels excessive.

### Creating the System Directory

Create a directory structure following the established pattern:

```
systems/my-service/
├── compose.yml        # Docker Compose service definition
├── config/            # Configuration files (mounted into container)
├── entrypoint.sh      # (optional) Initialization script
└── README.md          # Documentation for this system
```

The `config/` directory is optional but recommended. Configuration files mounted from here can be edited without rebuilding images—a convenience you'll appreciate during development.

### Compose File Template

Here's a working template that follows the conventions used throughout Smelt:

```yaml
# My Service System - Brief description
#
# Longer description of what this service does and why it exists.
# Used by: list services that depend on this one

services:
  my-service:
    image: myorg/my-service:dev
    ports:
      - "XXXX:80"  # Host:Container - describe what this exposes
    volumes:
      - my-service-data:/data
      - ../../generated/keys/my-service.pem:/keys/my-service.pem:ro
      - ./config:/config:ro
    environment:
      - LOG_LEVEL=info
      - MY_SERVICE_DID=did:web:my-service
    healthcheck:
      test: ["CMD", "curl", "-sf", "http://localhost:80/health"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 10s
    depends_on:
      some-dependency:
        condition: service_healthy
    restart: unless-stopped
    networks:
      - storacha-network

volumes:
  my-service-data:
```

A few notes on conventions:

- **Internal port 80**: Services that need `did:web` resolution should listen on port 80 internally. The `did:web` specification defaults to port 80, so `did:web:my-service` resolves to `http://my-service:80/.well-known/did.json` within the Docker network.

- **Read-only mounts**: Use `:ro` for keys and config files. This prevents accidental writes and makes your intentions clear.

- **Health checks**: Required for any service that others depend on. The `depends_on` condition `service_healthy` doesn't work without them.

- **start_period**: Services that need initialization time (piri takes nearly three minutes) should set this generously. The health check won't mark the container unhealthy during this period.

### Including in Root Compose

Add your system to the root `compose.yml`:

```yaml
include:
  - path: systems/blockchain/compose.yml
  - path: systems/common/compose.yml
  - path: systems/signing-service/compose.yml
  - path: systems/delegator/compose.yml
  - path: systems/indexing/compose.yml
  - path: systems/piri/compose.yml
  - path: systems/upload/compose.yml
  - path: systems/guppy/compose.yml
  - path: systems/my-service/compose.yml  # Add your service here
```

Order matters only insofar as it affects readability. Docker Compose resolves dependencies from the `depends_on` declarations, not from include order.

### Network Connectivity

All services connect to `storacha-network`, an external Docker network created during `make init`. This provides:

- **DNS resolution**: Service names are resolvable as hostnames. From any container, `http://piri:3000` reaches piri.
- **Isolation**: Only services on this network can communicate. Your host machine accesses services through published ports.

Example cross-service communication:

```yaml
environment:
  - PIRI_ENDPOINT=http://piri:3000
  - INDEXER_ENDPOINT=http://indexer:80
  - BLOCKCHAIN_RPC=ws://blockchain:8545
```

If your service needs to be reached by others, ensure it binds to `0.0.0.0` (not `localhost` or `127.0.0.1`).

### Generating Service Keys

If your service needs an Ed25519 identity key, add it to `generated/generate-keys.sh`. The pattern is:

```bash
generate_ed25519_key "my-service"
```

This creates `generated/keys/my-service.pem` and `generated/keys/my-service.pub`. Mount the `.pem` file into your container.

After modifying the key generation script, run:

```bash
make regen
make clean && make up
```

## Customizing Configuration

### Environment Variables

The `.env` file in the project root overrides default values. Copy the example:

```bash
cp .env.example .env
```

Available variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `ANVIL_BLOCK_TIME` | 3 | Seconds between blockchain blocks |
| `UPLOAD_PORT` | 8080 | External port for upload service |
| `UPLOAD_LOG_LEVEL` | info | Upload service verbosity |
| `INDEXER_PORT` | 9000 | External port for indexer |
| `INDEXER_LOG_LEVEL` | info | Indexer verbosity |
| `PIRI_PORT` | 3000 | Piri's internal port |
| `PIRI_LOG_LEVEL` | info | Piri verbosity |

Variables are referenced in compose files using `${VARIABLE:-default}` syntax.

### Service-Specific Configuration

Each service has configuration files in `systems/<service>/config/`:

| File | Service | Format |
|------|---------|--------|
| `piri-base-config.toml` | piri | TOML |
| `delegator.yaml` | delegator | YAML |
| `guppy-config.toml` | guppy | TOML |
| `signer.yaml` | signing-service | YAML |

These files are mounted read-only into containers. Edit them, then restart the affected service:

```bash
docker compose restart piri
```

For changes to take effect without restart, some services support configuration reload signals—check individual service documentation.

### Contract Addresses

Smart contract addresses are baked into `systems/blockchain/state/deployed-addresses.json` and must match values in service configs. If you deploy new contracts, update:

1. `systems/piri/config/piri-base-config.toml` — The `[pdp.contracts]` section
2. `systems/delegator/config/delegator.yaml` — The `contract` section
3. `systems/signing-service/compose.yml` — The `--service-contract-address` argument

The default addresses work with the pre-deployed state in `systems/blockchain/state/anvil-state.json`.

## Adding Network Simulation

Network problems are inevitable in distributed systems. Simulating them locally is faster than waiting for production to surface issues.

### Using Toxiproxy

Toxiproxy sits between services and introduces configurable failures: latency, packet loss, bandwidth throttling, connection resets.

Create the system directory:

```
systems/network-chaos/
├── compose.yml
└── toxiproxy.json
```

**compose.yml**:

```yaml
# Network Chaos System - Toxiproxy for network simulation
#
# Introduces controllable network failures between services.
# Useful for testing retry logic, timeouts, and resilience.

services:
  toxiproxy:
    image: ghcr.io/shopify/toxiproxy:2.7.0
    ports:
      - "8474:8474"   # Toxiproxy API
      - "3334:3334"   # Proxy to piri
      - "9001:9001"   # Proxy to indexer
    volumes:
      - ./toxiproxy.json:/config/toxiproxy.json:ro
    command: ["-config", "/config/toxiproxy.json"]
    networks:
      - storacha-network
```

**toxiproxy.json**:

```json
[
  {
    "name": "piri-proxy",
    "listen": "0.0.0.0:3334",
    "upstream": "piri:3000"
  },
  {
    "name": "indexer-proxy",
    "listen": "0.0.0.0:9001",
    "upstream": "indexer:80"
  }
]
```

### Applying Toxics

With Toxiproxy running, apply network conditions via its API:

```bash
# Add 200ms latency to piri requests
curl -X POST http://localhost:8474/proxies/piri-proxy/toxics \
  -H "Content-Type: application/json" \
  -d '{"name":"latency","type":"latency","attributes":{"latency":200}}'

# Add 10% packet loss (connections dropped)
curl -X POST http://localhost:8474/proxies/piri-proxy/toxics \
  -H "Content-Type: application/json" \
  -d '{"name":"timeout","type":"timeout","attributes":{"timeout":0},"toxicity":0.1}'

# Limit bandwidth to 1KB/s (simulates slow networks)
curl -X POST http://localhost:8474/proxies/piri-proxy/toxics \
  -H "Content-Type: application/json" \
  -d '{"name":"bandwidth","type":"bandwidth","attributes":{"rate":1}}'

# Remove a toxic
curl -X DELETE http://localhost:8474/proxies/piri-proxy/toxics/latency

# Reset all toxics on a proxy
curl -X POST http://localhost:8474/proxies/piri-proxy/toxics/populate -d '[]'
```

### Routing Traffic Through Toxiproxy

Configure clients to connect through the proxy instead of directly. For example, modify `guppy-config.toml` to use the proxy port:

```toml
[network]
# Instead of http://piri:3000, route through toxiproxy
upload_url = "http://toxiproxy:3334"
```

## Connecting to External Services

Smelt defaults to entirely local services. Sometimes you need to test against real infrastructure.

### Using a Real Blockchain

To connect piri and signing-service to an external EVM chain:

1. **Update signing-service** in `systems/signing-service/compose.yml`:

```yaml
command: [
  "--host", "0.0.0.0",
  "--port", "7446",
  "--rpc-url", "wss://your-chain-endpoint.example.com",  # External RPC
  "--service-contract-address", "0x...",  # Deployed contract address
  "--signing-key-path", "/keys/payer-key.hex",
  "--service-key-file", "/keys/signing-service.pem",
  "--service-did", "did:web:signing-service"
]
```

2. **Update piri config** in `systems/piri/config/piri-base-config.toml`:

```toml
[pdp]
chain_id = "ACTUAL_CHAIN_ID"  # e.g., "314159" for Filecoin Calibration
payer_address = "0x..."

[pdp.contracts]
verifier = "0x..."
provider_registry = "0x..."
service = "0x..."
service_view = "0x..."
payments = "0x..."
usdfc_token = "0x..."
```

3. **Update delegator config** in `systems/delegator/config/delegator.yaml`:

```yaml
contract:
  chain_client_endpoint: "wss://your-chain-endpoint.example.com"
  payments_contract_address: "0x..."
  service_contract_address: "0x..."
  registry_contract_address: "0x..."
  transactor:
    chain_id: ACTUAL_CHAIN_ID
    key: "0x..."  # Funded wallet private key
```

4. **Ensure your wallet is funded** on the target chain. PDP operations cost gas.

5. **Stop local blockchain** by commenting it out of `compose.yml` or removing its depends_on references.

### Using External IPNI

To query production IPNI for content discovery:

Update `systems/indexing/indexer/compose.yml`:

```yaml
environment:
  - IPNI_ENDPOINT=https://cid.contact
```

Note that you cannot announce to production IPNI from a local piri—production IPNI won't accept advertisements from localhost. This configuration is for reading only.

## Building and Pushing Custom Images

### Building Locally

```bash
# Build all images (only those with build contexts)
make build

# Build specific service
docker compose build upload

# Build with no cache (when you suspect caching issues)
docker compose build --no-cache upload

# Build with build arguments
docker compose build --build-arg VERSION=1.2.3 piri
```

### Using Custom Image Tags

Parameterize image tags in compose files:

```yaml
services:
  my-service:
    image: myorg/my-service:${MY_SERVICE_TAG:-dev}
```

Then run with a specific version:

```bash
MY_SERVICE_TAG=v1.2.3 make up
```

Or set it in your `.env` file for persistence.

### Pushing to a Registry

```bash
# Tag with your registry
docker tag smelt-upload:latest ghcr.io/yourorg/upload:dev

# Push (ensure you're logged in: docker login ghcr.io)
docker push ghcr.io/yourorg/upload:dev
```

### Using Local Builds of Service Repositories

If you're developing a service (piri, guppy, indexer) and want to test local changes:

1. Build the image locally in that repository
2. Tag it with the name Smelt expects:

```bash
# In piri repository
docker build -t forreststoracha/piri:dev .

# Or use docker compose build if piri's compose.yml has a build context
```

3. Restart Smelt to pick up the new image:

```bash
make down && make up
```

## Running Individual Systems Standalone

Each system can run independently if its dependencies are available. This is useful for focused testing.

```bash
# Just blockchain (no dependencies)
cd systems/blockchain && docker compose up -d

# Just common services (DynamoDB)
cd systems/common && docker compose up -d

# Indexing stack (needs redis, which is included)
cd systems/indexing && docker compose up -d
```

Note that most systems declare `external: true` for `storacha-network`, so you must create it first:

```bash
docker network create storacha-network
```

Or run `make init` once to handle all setup.

### Dependency Map

For standalone operation, ensure these dependencies are running:

| System | Requires |
|--------|----------|
| blockchain | — |
| common (dynamodb-local) | — |
| signing-service | blockchain |
| delegator | blockchain, dynamodb-local |
| indexing (ipni + indexer) | redis (included) |
| piri | blockchain, signing-service, delegator, indexer, dynamodb-local |
| upload | piri, indexer, dynamodb-local |
| guppy | upload, piri |

## Adding a New Storage Provider (Second Piri Instance)

Running multiple piri instances tests multi-provider scenarios: content replication, provider selection, failover.

### Create a Second Piri System

Create `systems/piri-2/` with modified configuration:

**compose.yml**:

```yaml
# Piri-2 System - Second storage provider
#
# A second piri instance for multi-provider testing.

services:
  piri-2:
    image: forreststoracha/piri:dev
    ports:
      - "3335:3000"  # Different host port
    volumes:
      - piri-2-data:/data/piri
      - ../../generated/keys/piri-2.pem:/keys/piri.pem:ro
      - ../../generated/keys/owner-wallet.hex:/keys/owner-wallet.hex:ro
      - ./entrypoint.sh:/entrypoint.sh:ro
      - ./register-did.sh:/scripts/register-did.sh:ro
      - ./config/piri-base-config.toml:/config/piri-base-config.toml:ro
    entrypoint: ["/entrypoint.sh"]
    environment:
      - DYNAMODB_ENDPOINT=http://dynamodb-local:8000
      - DYNAMODB_TABLE=delegator-allow-list
      - OPERATOR_EMAIL=local-2@test.com
      - PUBLIC_URL=http://piri-2:3000
      - PIRI_DISABLE_ANALYTICS=1
    depends_on:
      blockchain:
        condition: service_healthy
      indexer:
        condition: service_healthy
      signing-service:
        condition: service_healthy
      delegator:
        condition: service_healthy
      dynamodb-local:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "curl", "-sf", "http://localhost:3000/"]
      interval: 10s
      timeout: 5s
      retries: 30
      start_period: 180s
    restart: unless-stopped
    networks:
      - storacha-network

volumes:
  piri-2-data:
```

### Generate a New Key

Add to `generated/generate-keys.sh`:

```bash
generate_ed25519_key "piri-2"
```

Then regenerate:

```bash
./generated/generate-keys.sh --force
```

### Copy Support Scripts

Copy `entrypoint.sh` and `register-did.sh` from the original piri system:

```bash
cp systems/piri/entrypoint.sh systems/piri-2/
cp systems/piri/register-did.sh systems/piri-2/
```

Modify `entrypoint.sh` if needed (the `PUBLIC_URL` environment variable handles the different endpoint).

### Include in Root Compose

```yaml
include:
  # ... existing includes ...
  - path: systems/piri-2/compose.yml
```

The second piri will automatically register with the delegator allow-list during initialization, using its distinct DID.

## Debugging Tips

### Verbose Logging

Most services respect `LOG_LEVEL` or equivalent environment variables:

```yaml
environment:
  - LOG_LEVEL=debug
  - RUST_LOG=debug  # For Rust services
```

Restart the service after changing:

```bash
docker compose restart piri
```

### Inspecting UCAN Invocations

At debug log level, services log UCAN invocations and receipts. Search for:

- `invocation` — incoming UCAN requests
- `receipt` — responses to invocations
- `capability` — specific permissions being invoked
- `delegation` — capability delegations being used

```bash
docker compose logs -f piri 2>&1 | grep -E "(invocation|receipt|capability)"
```

### Checking DynamoDB State

DynamoDB Local provides a web shell at http://localhost:8000/shell/

To list tables:

```javascript
var dynamodb = new AWS.DynamoDB({
  endpoint: 'http://localhost:8000',
  region: 'us-west-1'
});
dynamodb.listTables({}, function(err, data) {
  console.log(data.TableNames);
});
```

Or use the AWS CLI:

```bash
aws dynamodb list-tables --endpoint-url http://localhost:8000 --region us-west-1

aws dynamodb scan --table-name delegator-allow-list \
  --endpoint-url http://localhost:8000 --region us-west-1
```

### Inspecting Blockchain State

Query the local Anvil chain via JSON-RPC:

```bash
# Get latest block number
curl -s -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}' | jq

# Get chain ID
curl -s -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"eth_chainId","params":[],"id":1}' | jq

# Get account balance (replace address)
curl -s -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"eth_getBalance","params":["0x70997970C51812dc3A010C7d01b50e0d17dc79C8","latest"],"id":1}' | jq

# Get contract storage slot
curl -s -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"eth_getStorageAt","params":["0x0DCd1Bf9A1b36cE34237eEaFef220932846BCD82","0x0","latest"],"id":1}' | jq
```

### Inspecting Container State

```bash
# Shell into a running container
docker compose exec piri bash

# View container environment
docker compose exec piri env

# Check container filesystem
docker compose exec piri ls -la /data/piri

# View container logs with timestamps
docker compose logs -t piri | tail -100
```

### Network Debugging

```bash
# Test connectivity between containers
docker compose exec guppy curl -v http://piri:3000/

# Check DNS resolution
docker compose exec guppy nslookup piri

# List containers on the network
docker network inspect storacha-network --format '{{range .Containers}}{{.Name}} {{end}}'
```

### Resetting Individual Services

Sometimes a service gets into a bad state. Reset it without affecting others:

```bash
# Stop and remove container + volume
docker compose rm -sf piri
docker volume rm smelt_piri-data

# Restart just that service
docker compose up -d piri
```

For a complete reset, `make fresh` removes everything and rebuilds from scratch.

## Writing System Documentation

Each system should have a `README.md` following this structure:

```markdown
# System Name

Brief description of what this system does.

## Services

- **service-name** - What this service does

## Ports

| Port | Service | Description |
|------|---------|-------------|
| XXXX | service | What this port exposes |

## Configuration

- `config/file.toml` - What this configures

## Keys

- `../../generated/keys/service.pem` - What this key is for

## Volumes

- `volume-name` - What data this persists

## Dependencies

- dependency-name (service_healthy)

## Used By

- downstream-service
```

This consistency helps developers understand unfamiliar systems quickly.

## Summary

Smelt is designed to be modified. The conventions described here—system directories, compose patterns, network topology, key management—exist to make modifications predictable. When in doubt, examine how existing systems are structured and follow the established patterns.

The goal is a development environment that stays out of your way while you work on the interesting problems.
