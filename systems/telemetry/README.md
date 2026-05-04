# Telemetry System

Optional observability stack for Smelt local development. Provides metrics, distributed tracing, and dashboards. Included in the root compose file but gated behind the `telemetry` profile — it only starts when you opt in.

## Quick Start

```bash
# Start all services WITH the telemetry stack
make up-telemetry

# Open Grafana (anonymous admin, no login prompt)
make grafana       # opens http://localhost:15200
```

## Components

| Service        | Host Port | Purpose                                 |
|----------------|-----------|-----------------------------------------|
| Grafana        | 15200     | Dashboard visualization                 |
| Prometheus     | 15201     | Metrics storage                         |
| Tempo (HTTP)   | 15202     | Distributed tracing (query API)         |
| Tempo (gRPC)   | 15203     | Tempo gRPC                              |
| OTEL Collector | 15204/5   | Telemetry pipeline (OTLP gRPC/HTTP)     |
| OTEL metrics   | 15206     | Collector internal metrics              |
| OTEL Prom exp. | 15207     | Prometheus-format exporter              |
| Loki           | 15208     | Log aggregation (push + query API)      |
| Alloy          | 15209     | Log collector UI / debug endpoint       |
| cAdvisor       | 15210     | Per-container metrics (CPU/mem/net)     |

Container-internal ports are unchanged (Grafana listens on 3000, Prometheus on 9090, etc.). Only the host side of each mapping lives in the `152xx` range — keep that in mind when one service talks to another on the Docker network (e.g. `http://prometheus:9090`, not `:15201`).

## Architecture

```
Services (piri, ipni, upload, ...)           Docker daemon
          │                                          │
      ┌───┴───┐                           ┌──────────┴──────────┐
      │ OTLP  │                           │ stdout/stderr       │
      ▼       ▼                           ▼                     ▼
  ┌────────────────┐                  ┌───────┐           ┌──────────┐
  │ OTEL Collector │                  │ Alloy │           │ cAdvisor │
  └────────────────┘                  └───────┘           └──────────┘
     │         │                          │                     │
  ┌──┴──┐   ┌──┴──┐                       │                     │
  ▼     ▼   ▼     ▼                       ▼                     │
 ┌───────┐  ┌─────┐                   ┌───────┐                 │
 │ Prom. │  │Tempo│                   │ Loki  │                 │
 └───────┘  └─────┘                   └───────┘                 │
  ▲ ▲                                     │                     │
  │ └─────────────────────────────────────┼─────────────────────┘
  │  (cAdvisor is scraped by Prom,        │
  │   not routed through the collector)   │
  │                                       │
  └──────┬────────────────────────────────┘
         ▼
   ┌─────────┐
   │ Grafana │
   └─────────┘
```

**Four signals, four backends:**
- **Metrics (services)** → OTLP → OTEL Collector → Prometheus (remote-write)
- **Metrics (containers)** → cAdvisor → Prometheus (direct scrape)
- **Traces** → OTLP → OTEL Collector → Tempo
- **Logs** → stdout → Docker daemon → Alloy (via socket) → Loki

Why Alloy and not the OTEL Collector for logs? Alloy's `loki.source.docker` component auto-discovers every container from the Docker API, attaches compose labels (service, container, image) as Loki labels for free, and works identically on Linux and macOS Docker Desktop without any filesystem bind-mounts. The OTEL Collector's filelog receiver would need `/var/lib/docker/containers` mounted, which is broken on Docker Desktop.

**cAdvisor** (port 15210) is a separate track — it auto-discovers every running container via the Docker socket and emits per-container CPU/memory/network metrics to Prometheus on a direct scrape. This is the data source behind the "Smelt Service Fleet" dashboard ("what's running + how heavy is it?"). Labels include `container_label_com_docker_compose_project` and `container_label_com_docker_compose_service`, which are the primary query dimensions.

## Dashboards

Dashboards are provisioned from disk. The directory tree mirrors the Grafana folder layout:

```
systems/telemetry/config/grafana/dashboards/
├── dashboards.yaml              # provisioning config (foldersFromFilesStructure: true)
├── overview/                    # → "overview" folder in Grafana
│   ├── smelt-overview.json      #   Telemetry pipeline self-health
│   └── service-fleet.json       #   Fleet topology (cAdvisor-driven)
├── piri/                        # → "piri" folder
│   └── piri-node.json           #   Per-node view, switch via $DS dropdown
├── upload/                      # → "upload" folder
│   └── sprue-postgres.json      #   Sprue metadata store
└── indexing/                    # → "indexing" folder (empty — reserved)
```

Adding a new service is just `mkdir systems/telemetry/config/grafana/dashboards/<service>` — the next Grafana rescan picks up the new folder.

**What each dashboard is for:**

- **Smelt Service Fleet** (`smelt-fleet`) — the "shape of my stack" view. Fleet grid (one tile per container, colored by liveness), CPU/memory/network per service, full container inventory table. Starts here when you want to know what's running and how heavy it's breathing.
- **Smelt Overview** (`smelt-overview`) — telemetry pipeline self-health. Is the OTEL Collector ingesting? How many spans per second? Useful when telemetry itself seems broken.
- **Sprue Postgres** (`sprue-postgres`) — data-plane view of the upload service: throughput, storage, UCAN delegations, agent index. Data from sprue's own postgres.
- **Piri Storage Node** (`piri-node`) — per-node view into piri's job queues (aggregator / replicator / egress_tracker) and scheduler (PDP + blockchain). Use the `$DS` dropdown to switch between piri-0 / piri-1 / piri-2.

### Editing dashboards

Two workflows are supported. **Pick one at a time per dashboard** — they don't mix cleanly.

**1. Edit on disk (recommended for iteration on a committed dashboard).** Open the JSON file in your editor, save, and Grafana picks up the change within 30 seconds (the `updateIntervalSeconds` in `dashboards.yaml`). Refresh your browser tab to see it. Any UI-only edits on the same dashboard are overwritten on the next rescan.

**2. Edit in the UI (recommended for prototyping a new dashboard).** Build the dashboard in Grafana, save it to the appropriate folder (e.g. "piri"), then:

```bash
make grafana-export
```

This runs `systems/telemetry/scripts/grafana-export.sh`, which curls the Grafana API and writes:

- every dashboard to `config/grafana/dashboards/<folder-slug>/<title-slug>.json`
- all unified-alerting config to `config/grafana/alerting/{rules,contact-points,policies}.yaml`

The export is idempotent. Review the diff, then:

```bash
git add systems/telemetry/config/grafana
git commit -m "telemetry: dashboards for <feature>"
```

On the next fresh boot (`make clean && make up-telemetry`), your committed dashboards + alerts are provisioned automatically.

### Alerts

Unified alerting (the modern Grafana alerting system, not legacy) is provisioned from `config/grafana/alerting/`. Create alert rules in the UI, then `make grafana-export` pulls them into `rules.yaml` alongside any contact points and notification policies. The files are hot-reloaded on the same 30s interval as dashboards.

## Logs

Alloy streams the stdout/stderr of every container in the `smelt` compose project to Loki via the Docker daemon API — no per-service instrumentation is needed. Labels attached to each log stream:

- `service` — the compose service name (e.g. `piri-0`, `upload`, `ipni`)
- `container` — the full container name
- `image` — image including tag
- `source=docker`, `cluster=smelt`

### Querying

In Grafana, go to **Explore** → select the **Loki** datasource. Example LogQL queries:

```logql
# All logs from a specific service
{service="piri-0"}

# All logs from any piri node
{service=~"piri-[0-9]+"}

# Errors across the whole stack (case-insensitive)
{cluster="smelt"} |~ "(?i)error|fatal|panic"

# Just sprue, JSON-parsed, filtered to request logs
{service="upload"} | json | level="INFO" | line_format "{{.method}} {{.uri}} → {{.status}}"

# Rate of log lines per service, last 5m
sum by (service) (rate({cluster="smelt"}[5m]))
```

### Trace correlation

When a log line contains `trace_id=<hex>` (piri and sprue both emit structured logs with these when OTLP tracing is enabled), Grafana's Loki datasource renders a **"View trace"** link next to the line that jumps to the matching trace in Tempo. Tempo spans also get a "Logs for this span" button via the `tracesToLogsV2` datasource link.

### Retention

Default: **7 days**. Loki's compactor deletes chunks beyond the retention window every 10 minutes. Adjust in `config/loki/loki.yaml` under `limits_config.retention_period`.

### Feedback loop note

Alloy's own logs are explicitly **dropped** (not shipped to Loki) by a rule in `config/alloy/config.alloy`. Otherwise, every log line Alloy itself writes about ingesting log lines would feed back through Alloy and blow up Loki.

## Configuring Services

When `make up-telemetry` is used, `OTEL_ENABLED=true` and `OTEL_ENDPOINT=http://otel-collector:4318` are exported. Per-service compose files read those vars and enable their telemetry exporters (see e.g. `systems/indexing/ipni/compose.yml`). For services that aren't wired up yet, the minimal change is:

```yaml
environment:
  - OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4317
  - OTEL_SERVICE_NAME=my-service
```

## Endpoints

| Endpoint      | URL                    | Description                   |
|---------------|------------------------|-------------------------------|
| Grafana UI    | http://localhost:15200 | Dashboards (anonymous admin)  |
| Prometheus UI | http://localhost:15201 | Metrics query                 |
| Tempo API     | http://localhost:15202 | Trace query                   |
| OTLP gRPC     | localhost:15204        | Send telemetry (gRPC)         |
| OTLP HTTP     | localhost:15205        | Send telemetry (HTTP)         |
| Loki API      | http://localhost:15208 | LogQL query / push API        |
| Alloy UI      | http://localhost:15209 | Scrape status + config debug  |
| cAdvisor UI   | http://localhost:15210 | Per-container metrics browser |

## Resource Usage

- ~500MB–1GB RAM for the full stack
- Minimal CPU when idle
- Disk grows with retention (48h default for traces)

## Cleanup

```bash
make down     # stop everything (telemetry included; --remove-orphans catches profile services)

# Remove telemetry volumes (delete all stored metrics, traces, logs, dashboards DB):
docker volume rm smelt_prometheus-data smelt_tempo-data smelt_grafana-data smelt_loki-data
```
