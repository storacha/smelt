SHELL := /bin/bash
DOCKER := $(shell which docker)

# Set YES=1 to skip confirmation prompts (e.g., make nuke YES=1)
YES ?= 0

.PHONY: help generate init up up-telemetry down restart clean nuke fresh logs pull build status guppy regen debug-upload ensure-state check-docker grafana grafana-export

# Default target - show help
help:
	@echo "Storacha Compose - Local Development Environment"
	@echo ""
	@echo "Quick Start:"
	@echo "  make up        Start the network (initializes if needed)"
	@echo "  make down      Stop the network (keeps data)"
	@echo "  make restart   Restart all services"
	@echo ""
	@echo "Lifecycle:"
	@echo "  make generate  Generate compose files and keys from smelt.yml"
	@echo "  make init      Initialize keys, proofs, and Docker network"
	@echo "  make up        Start all services"
	@echo "  make down      Stop all services (preserves data)"
	@echo "  make restart   Stop and start all services"
	@echo "  make clean     Stop + delete volumes (DESTROYS ALL DATA)"
	@echo "  make nuke      Stop + delete volumes + keys + images (DESTROYS EVERYTHING)"
	@echo "  make fresh     Nuke + rebuild + start (DESTROYS EVERYTHING, starts fresh)"
	@echo ""
	@echo "Piri Configuration:"
	@echo "  Edit smelt.yml to configure piri node count and storage backends."
	@echo "  Run 'make generate' (or 'make up') to apply changes."
	@echo ""
	@echo "Snapshots:"
	@echo "  ./smelt snapshot save NAME        Save current stack state"
	@echo "  ./smelt snapshot list             List saved snapshots"
	@echo "  ./smelt snapshot rm NAME          Delete a snapshot"
	@echo "  make up SNAPSHOT=NAME             Boot from a snapshot (or /path/to/snapshot)"
	@echo "  See docs/SNAPSHOTS.md for the full picture"
	@echo ""
	@echo "Development:"
	@echo "  make pull          Pull latest pre-built images"
	@echo "  make build         Build all Docker images"
	@echo "  make regen         Regenerate keys and proofs (requires restart)"
	@echo "  make logs          Follow all service logs"
	@echo "  make status        Show service status"
	@echo "  make shell-guppy   Open shell in guppy container"
	@echo ""
	@echo "Debugging:"
	@echo "  make debug-upload  Run upload (sprue) under Delve on localhost:2345"
	@echo ""
	@echo "Telemetry (Grafana / Prometheus / Tempo / OTEL Collector):"
	@echo "  make up-telemetry     Start all services WITH the telemetry stack"
	@echo "  make grafana          Open Grafana (http://localhost:15200)"
	@echo "  make grafana-export   Dump Grafana dashboards + alerts to config/ for git commit"
	@echo ""
	@echo "Options:"
	@echo "  YES=1              Skip confirmation prompts (e.g., make nuke YES=1)"
	@echo ""
	@echo "Destructive commands (clean, nuke, fresh) require confirmation."
	@echo ""

# Fail early if docker engine is too old. Smelt relies on features added in
# engine 25 (healthcheck.start_interval, compose top-level `name:`). On older
# engines, start_interval is silently ignored and snapshot-restored boots are
# ~3x slower than they should be.
check-docker:
	@version=$$(docker version --format '{{.Server.Version}}' 2>/dev/null); \
	major=$$(echo "$$version" | cut -d. -f1); \
	if [ -z "$$major" ]; then \
		echo "ERROR: could not determine docker engine version"; \
		echo "       is the docker daemon running?"; \
		exit 1; \
	fi; \
	if [ "$$major" -lt 25 ]; then \
		echo "ERROR: docker engine $$version is below the required minimum of 25.0"; \
		echo "       Upgrade: https://docs.docker.com/engine/install/"; \
		exit 1; \
	fi

# Seed generated/snapshot-scratch/ from the committed post-deploy baseline
# when no working chain state exists yet. After first seed, the SIGTERM dump
# from the blockchain container keeps these files current across down/up
# cycles; we never overwrite in place. `make clean` / `make nuke` clears
# scratch so the next `make up` picks up the baseline again.
ensure-state: check-docker
	@mkdir -p generated/snapshot-scratch
	@# Self-heal: if a prior compose up found a non-existent bind-mount source
	@# and docker auto-created it as a directory, remove the dir so we can
	@# seed a file in its place.
	@[ -d generated/snapshot-scratch/anvil-state.json ] && rm -rf generated/snapshot-scratch/anvil-state.json || true
	@[ -d generated/snapshot-scratch/deployed-addresses.json ] && rm -rf generated/snapshot-scratch/deployed-addresses.json || true
	@if [ ! -f generated/snapshot-scratch/anvil-state.json ] && [ -f systems/blockchain/state/anvil-state.json ]; then \
		cp systems/blockchain/state/anvil-state.json generated/snapshot-scratch/anvil-state.json; \
	fi
	@if [ ! -f generated/snapshot-scratch/deployed-addresses.json ] && [ -f systems/blockchain/state/deployed-addresses.json ]; then \
		cp systems/blockchain/state/deployed-addresses.json generated/snapshot-scratch/deployed-addresses.json; \
	fi

# Generate compose files and keys from smelt.yml manifest
generate:
	@go run ./cmd/smelt generate

# File target: rebuild the generated piri compose when the manifest or
# generator source changes. Compose-invoking targets below depend on this
# so fresh checkouts and post-nuke states regenerate piri.yml on demand.
generated/compose/piri.yml: smelt.yml $(shell find cmd/smelt pkg/generate pkg/manifest -name '*.go' 2>/dev/null)
	@go run ./cmd/smelt generate

# Initialize the environment (generate keys, proofs, create network)
init: generate
	@./scripts/init.sh

# Start all services (runs init first if needed).
#
# Pass SNAPSHOT=<name-or-path> to load a snapshot before starting — keys,
# proofs, blockchain state, docker volumes, and a session manifest at
# generated/snapshot-scratch/smelt.yml are all populated from it. The
# project's tracked smelt.yml is never touched; subsequent `make up` calls
# (with or without SNAPSHOT) stay on the session manifest until `make clean`
# or `make nuke` removes it.
up: ensure-state
	@if [ -n "$(SNAPSHOT)" ]; then \
		echo "Loading snapshot: $(SNAPSHOT)"; \
		go run ./cmd/smelt snapshot load "$(SNAPSHOT)"; \
	fi
	@if [ ! -d "generated/keys" ] || [ -z "$$(ls -A generated/keys 2>/dev/null)" ]; then \
		$(MAKE) init; \
	else \
		$(MAKE) generate; \
	fi
	$(DOCKER) compose up -d --remove-orphans
	@echo ""
	@echo "Services starting. Run 'make status' to check health."
	@echo "Run 'make logs' to follow logs."

# Start all services WITH the telemetry stack. The telemetry services
# (Grafana / Prometheus / Tempo / OTEL Collector) are gated behind the
# `telemetry` compose profile, so we opt in via COMPOSE_PROFILES.
# OTEL_ENABLED / OTEL_ENDPOINT are consumed by per-service compose files
# (e.g. systems/indexing/ipni/compose.yml) that can emit telemetry when
# the collector is reachable.
up-telemetry:
	@COMPOSE_PROFILES=telemetry \
	 OTEL_ENABLED=true \
	 OTEL_ENDPOINT=http://otel-collector:4318 \
	 $(MAKE) up
	@echo ""
	@echo "Telemetry stack started:"
	@echo "  Grafana UI    http://localhost:15200"
	@echo "  Prometheus UI http://localhost:15201"
	@echo "  Tempo API     http://localhost:15202"

# Open the Grafana UI in the default browser (best-effort; prints URL otherwise)
grafana:
	@url=http://localhost:15200; \
	if command -v xdg-open >/dev/null 2>&1; then xdg-open "$$url" >/dev/null 2>&1; \
	elif command -v open >/dev/null 2>&1; then open "$$url"; \
	else echo "Open $$url in your browser"; fi

# Export dashboards + unified-alerting config from the running Grafana back
# to systems/telemetry/config/grafana/ so the result can be committed to git.
# Requires the telemetry stack to be running (make up-telemetry).
grafana-export:
	@./systems/telemetry/scripts/grafana-export.sh

# Stop all services (keeps volumes for quick restart).
# `--profile "*"` includes profile-gated services (telemetry) that would
# otherwise be considered inactive and skipped by `down`.
down: generated/compose/piri.yml ensure-state
	$(DOCKER) compose --profile '*' down --remove-orphans
	@echo ""
	@echo "Services stopped. Data preserved in volumes."
	@echo "Run 'make up' to restart."

# Restart all services
restart: down up

# Helper to confirm destructive operations
define confirm
	@if [ "$(YES)" != "1" ]; then \
		echo ""; \
		echo "WARNING: This will $(1)"; \
		echo ""; \
		read -p "Are you sure? [y/N] " confirm && [ "$$confirm" = "y" ] || (echo "Aborted." && exit 1); \
	fi
endef

# Stop services and remove volumes (but keep keys/proofs)
clean: generated/compose/piri.yml check-docker
	$(call confirm,STOP all services and DELETE all volumes (Redis cache$(,) IPNI data$(,) etc.))
	@# `--profile "*"` catches profile-gated services (telemetry) that
	@# `down` alone would skip.
	$(DOCKER) compose --profile '*' down -v --remove-orphans
	@# Also remove any dangling volumes from this project
	$(DOCKER) volume ls -q --filter "name=smelt_" | xargs -r $(DOCKER) volume rm 2>/dev/null || true
	@# Clear chain state so next `make up` cold-boots from the committed baseline.
	@# Leaving it would produce a half-warm stack: empty volumes but a mutated chain.
	rm -rf generated/snapshot-scratch/anvil-state.json generated/snapshot-scratch/deployed-addresses.json
	@# End any active snapshot session so `make up` goes back to project smelt.yml.
	rm -f generated/snapshot-scratch/smelt.yml
	@echo ""
	@echo "Services stopped, volumes removed, chain state reset."
	@echo "Keys and proofs preserved. Run 'make up' to restart."

# Remove EVERYTHING - volumes, keys, proofs, and built images
nuke: generated/compose/piri.yml check-docker
	$(call confirm,DELETE everything: containers$(,) volumes$(,) keys$(,) proofs$(,) AND Docker images)
	@echo "Removing all containers, volumes, keys, proofs, and images..."
	@# `--profile "*"` catches profile-gated services (telemetry) that
	@# `down` alone would skip.
	$(DOCKER) compose --profile '*' down -v --remove-orphans --rmi local 2>/dev/null || true
	@# Also remove any dangling volumes from this project
	$(DOCKER) volume ls -q --filter "name=smelt_" | xargs -r $(DOCKER) volume rm 2>/dev/null || true
	rm -rf generated/keys generated/proofs generated/compose
	rm -rf generated/snapshot-scratch/anvil-state.json generated/snapshot-scratch/deployed-addresses.json
	rm -f generated/snapshot-scratch/smelt.yml
	@echo ""
	@echo "Everything removed. Run 'make up' or 'make fresh' to start over."

# Complete fresh start - nuke everything, rebuild, and start
fresh: generated/compose/piri.yml check-docker
	$(call confirm,DELETE everything and rebuild from scratch)
	@echo "Removing all containers, volumes, keys, proofs, and images..."
	@# `--profile "*"` catches profile-gated services (telemetry) that
	@# `down` alone would skip.
	$(DOCKER) compose --profile '*' down -v --remove-orphans --rmi local 2>/dev/null || true
	@# Also remove any dangling volumes from this project
	$(DOCKER) volume ls -q --filter "name=smelt_" | xargs -r $(DOCKER) volume rm 2>/dev/null || true
	rm -rf generated/keys generated/proofs generated/compose
	rm -rf generated/snapshot-scratch/anvil-state.json generated/snapshot-scratch/deployed-addresses.json
	rm -f generated/snapshot-scratch/smelt.yml
	@echo ""
	@echo "Rebuilding and starting fresh..."
	$(MAKE) init
	$(MAKE) ensure-state
	$(DOCKER) compose build
	$(DOCKER) compose up -d --remove-orphans
	@echo ""
	@echo "Fresh deployment complete!"
	@echo ""
	@echo "Next steps:"
	@echo "  make shell-guppy       Open guppy shell"
	@echo "  guppy login EMAIL      Create account"
	@echo "  guppy space generate   Create a space"

# Regenerate keys and proofs (requires service restart to take effect)
regen:
	@echo "Regenerating keys and proofs..."
	go run ./cmd/smelt generate --force
	./generated/generate-proofs.sh --force
	@echo ""
	@echo "Keys and proofs regenerated."
	@echo "Run 'make clean && make up' to restart services with new keys."

# Pull latest pre-built images (ignores failures for local-only images)
pull: generated/compose/piri.yml ensure-state
	$(DOCKER) compose pull --ignore-pull-failures

# Build all images
build: generated/compose/piri.yml ensure-state
	$(DOCKER) compose build

# Follow logs from all services
logs: generated/compose/piri.yml ensure-state
	$(DOCKER) compose logs -f

# Show service status
status: generated/compose/piri.yml ensure-state
	@$(DOCKER) compose ps
	@echo ""
	@$(DOCKER) compose ps --format "table {{.Name}}\t{{.Status}}" | grep -E "(healthy|unhealthy|starting)" || true

# Shell into guppy container
shell-guppy: generated/compose/piri.yml ensure-state
	$(DOCKER) compose exec guppy bash

# Shell into piri-0 container
shell-piri: generated/compose/piri.yml ensure-state
	$(DOCKER) compose exec piri-0 sh

# Shell into upload container
shell-upload: ensure-state
	$(DOCKER) compose exec upload bash

# Run upload (sprue) under Delve for remote debugging.
# See compose.debug.yml for the overlay; attach to localhost:2345.
debug-upload: generated/compose/piri.yml ensure-state
	@if [ ! -d "generated/keys" ] || [ -z "$$(ls -A generated/keys 2>/dev/null)" ]; then \
		$(MAKE) init; \
	fi
	$(DOCKER) compose -f compose.yml -f compose.debug.yml up -d --force-recreate upload
	@echo ""
	@echo "upload is running under Delve. Attach to localhost:2345:"
	@echo "  dlv connect localhost:2345"
	@echo "  (or VS Code 'Connect to server' / GoLand 'Go Remote')"
