SHELL := /bin/bash
DOCKER := $(shell which docker)

# Set YES=1 to skip confirmation prompts (e.g., make nuke YES=1)
YES ?= 0

# Profile support: pass PROFILES="profile1 profile2" to enable profiles
# Example: make fresh PROFILES="piri-postgres piri-s3"
PROFILES ?=
PROFILE_FLAGS := $(foreach p,$(PROFILES),--profile $(p))

# Auto-set environment variables based on profiles
PROFILE_ENV :=
ifneq (,$(findstring piri-postgres,$(PROFILES)))
    PROFILE_ENV += PIRI_DB_BACKEND=postgres
endif
ifneq (,$(findstring piri-s3,$(PROFILES)))
    PROFILE_ENV += PIRI_BLOB_BACKEND=s3
endif

.PHONY: help init up down restart clean nuke fresh logs pull build status guppy regen up-telemetry grafana telemetry-status stress shell-stress stress-status up-piri-postgres up-piri-s3 up-piri-postgres-s3

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
	@echo "  make init      Initialize keys, proofs, and Docker network"
	@echo "  make up        Start all services"
	@echo "  make down      Stop all services (preserves data)"
	@echo "  make restart   Stop and start all services"
	@echo "  make clean     Stop + delete volumes (DESTROYS ALL DATA)"
	@echo "  make nuke      Stop + delete volumes + keys + images (DESTROYS EVERYTHING)"
	@echo "  make fresh     Nuke + rebuild + start (DESTROYS EVERYTHING, starts fresh)"
	@echo ""
	@echo "Development:"
	@echo "  make pull      Pull latest pre-built images"
	@echo "  make build     Build all Docker images"
	@echo "  make regen     Regenerate keys and proofs (requires restart)"
	@echo "  make logs      Follow all service logs"
	@echo "  make status    Show service status"
	@echo "  make guppy     Open shell in guppy container"
	@echo ""
	@echo "Telemetry:"
	@echo "  make up-telemetry       Start with observability stack (Grafana, Prometheus, Tempo)"
	@echo "  make grafana            Show Grafana URL and dashboard info"
	@echo "  make telemetry-status   Show telemetry service status"
	@echo ""
	@echo "Stress Testing:"
	@echo "  make stress             Start services with stress tester"
	@echo "  make shell-stress       Shell into stress tester container"
	@echo "  make stress-status      Show stress tester status and metrics"
	@echo ""
	@echo "Piri Storage Profiles:"
	@echo "  make up-piri-postgres      Start with PostgreSQL database backend"
	@echo "  make up-piri-s3            Start with S3 (MinIO) blob storage"
	@echo "  make up-piri-postgres-s3   Start with both PostgreSQL and S3"
	@echo ""
	@echo "Options:"
	@echo "  PROFILES=\"p1 p2\"  Enable profiles (e.g., make fresh PROFILES=\"piri-postgres piri-s3\")"
	@echo "  YES=1              Skip confirmation prompts (e.g., make nuke YES=1)"
	@echo ""
	@echo "Destructive commands (clean, nuke, fresh) require confirmation."
	@echo ""

# Initialize the environment (generate keys, proofs, create network)
init:
	@./scripts/init.sh

# Start all services (runs init first if needed)
# Use PROFILES="profile1 profile2" to enable profiles (e.g., make up PROFILES="piri-postgres")
up:
	@if [ ! -d "generated/keys" ] || [ -z "$$(ls -A generated/keys 2>/dev/null)" ]; then \
		$(MAKE) init; \
	fi
	$(PROFILE_ENV) $(DOCKER) compose $(PROFILE_FLAGS) up -d
	@echo ""
	@echo "Services starting. Run 'make status' to check health."
	@echo "Run 'make logs' to follow logs."

# Stop all services (keeps volumes for quick restart)
down:
	$(DOCKER) compose --profile telemetry --profile stress --profile piri-postgres --profile piri-s3 down
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
clean:
	$(call confirm,STOP all services and DELETE all volumes (Redis cache$(,) IPNI data$(,) etc.))
	@# Stop all services including those with profiles
	$(DOCKER) compose --profile telemetry --profile stress --profile piri-postgres --profile piri-s3 down -v --remove-orphans
	@# Also remove any dangling volumes from this project
	$(DOCKER) volume ls -q --filter "name=smelt_" | xargs -r $(DOCKER) volume rm 2>/dev/null || true
	@echo ""
	@echo "Services stopped and volumes removed."
	@echo "Keys and proofs preserved. Run 'make up' to restart."

# Remove EVERYTHING - volumes, keys, proofs, and built images
nuke:
	$(call confirm,DELETE everything: containers$(,) volumes$(,) keys$(,) proofs$(,) AND Docker images)
	@echo "Removing all containers, volumes, keys, proofs, and images..."
	@# Stop all services including those with profiles
	$(DOCKER) compose --profile telemetry --profile stress --profile piri-postgres --profile piri-s3 down -v --remove-orphans --rmi local 2>/dev/null || true
	@# Also remove any dangling volumes from this project
	$(DOCKER) volume ls -q --filter "name=smelt_" | xargs -r $(DOCKER) volume rm 2>/dev/null || true
	rm -rf generated/keys generated/proofs
	@echo ""
	@echo "Everything removed. Run 'make up' or 'make fresh' to start over."

# Complete fresh start - nuke everything, rebuild, and start
# Use PROFILES="profile1 profile2" to enable profiles (e.g., make fresh PROFILES="piri-postgres piri-s3")
fresh:
	$(call confirm,DELETE everything and rebuild from scratch)
	@echo "Removing all containers, volumes, keys, proofs, and images..."
	@# Stop all services including those with profiles
	$(DOCKER) compose --profile telemetry --profile stress --profile piri-postgres --profile piri-s3 down -v --remove-orphans --rmi local 2>/dev/null || true
	@# Also remove any dangling volumes from this project
	$(DOCKER) volume ls -q --filter "name=smelt_" | xargs -r $(DOCKER) volume rm 2>/dev/null || true
	rm -rf generated/keys generated/proofs
	@echo ""
	@echo "Rebuilding and starting fresh..."
	$(MAKE) init
	$(DOCKER) compose $(PROFILE_FLAGS) build
	$(PROFILE_ENV) $(DOCKER) compose $(PROFILE_FLAGS) up -d
	@echo ""
	@echo "Fresh deployment complete!"
	@echo ""
	@echo "Next steps:"
	@echo "  make shell-guppy           Open guppy shell"
	@echo "  guppy login EMAIL    Create account"
	@echo "  guppy space generate Create a space"

# Regenerate keys and proofs (requires service restart to take effect)
regen:
	@echo "Regenerating keys and proofs..."
	./generated/generate-keys.sh --force
	./generated/generate-proofs.sh --force
	@echo ""
	@echo "Keys and proofs regenerated."
	@echo "Run 'make clean && make up' to restart services with new keys."

# Pull latest pre-built images (ignores failures for local-only images)
pull:
	$(DOCKER) compose --profile telemetry --profile stress --profile piri-postgres --profile piri-s3 pull --ignore-pull-failures

# Build all images
build:
	$(DOCKER) compose --profile telemetry --profile stress --profile piri-postgres --profile piri-s3 build

# Follow logs from all services
logs:
	$(DOCKER) compose logs -f

# Show service status
status:
	@$(DOCKER) compose ps
	@echo ""
	@$(DOCKER) compose ps --format "table {{.Name}}\t{{.Status}}" | grep -E "(healthy|unhealthy|starting)" || true

# Shell into guppy container
shell-guppy:
	$(DOCKER) compose exec guppy bash

# Shell into piri container
shell-piri:
	$(DOCKER) compose exec piri sh

# Start with telemetry stack (Grafana, Prometheus, Tempo, OTEL Collector)
up-telemetry:
	@if [ ! -d "generated/keys" ] || [ -z "$$(ls -A generated/keys 2>/dev/null)" ]; then \
		$(MAKE) init; \
	fi
	@# Start telemetry services first so collectors are ready
	$(DOCKER) compose --profile telemetry up -d otel-collector prometheus tempo grafana
	@# Then start core services (they report to otel-collector via config)
	$(DOCKER) compose up -d
	@echo ""
	@echo "Services started with telemetry enabled."
	@echo "Grafana: http://localhost:3001"
	@echo ""
	@echo "Run 'make status' to check health."
	@echo "Run 'make telemetry-status' to check telemetry services."

# Show Grafana URL and info
grafana:
	@echo "Grafana: http://localhost:3001"
	@echo ""
	@echo "Access: Anonymous admin (no login required)"
	@echo ""
	@echo "Dashboards (Smelt folder):"
	@echo "  - Smelt Overview: System health and telemetry metrics"
	@echo ""
	@echo "Explore:"
	@echo "  - Prometheus: Query metrics"
	@echo "  - Tempo: Search traces"

# Show telemetry service status
telemetry-status:
	@echo "Telemetry Services:"
	@$(DOCKER) compose --profile telemetry ps --format "table {{.Name}}\t{{.Status}}" 2>/dev/null | grep -E "otel|prometheus|tempo|grafana" || echo "  (no telemetry services running - use 'make up-telemetry')"

# Start services with stress tester
stress:
	@if [ ! -d "generated/keys" ] || [ -z "$$(ls -A generated/keys 2>/dev/null)" ]; then \
		$(MAKE) init; \
	fi
	@# Start core services first if not running
	@$(DOCKER) compose up -d
	@# Then start stress-tester (uses profile to enable it)
	$(DOCKER) compose --profile stress up -d stress-tester
	@echo ""
	@echo "Stress tester started."
	@echo "Stress tester metrics: http://localhost:9091/metrics"
	@echo ""
	@echo "Run 'make shell-stress' to run tests manually."
	@echo "Run 'make stress-status' to check stress tester status."

# Shell into stress tester container
shell-stress:
	$(DOCKER) compose --profile stress exec stress-tester sh

# Show stress tester status
stress-status:
	@echo "Stress Tester Status:"
	@$(DOCKER) compose --profile stress ps --format "table {{.Name}}\t{{.Status}}" 2>/dev/null | grep -E "stress" || echo "  (stress tester not running - use 'make stress')"
	@echo ""
	@echo "Metrics endpoint: http://localhost:9091/metrics"
	@echo ""
	@echo "View logs: docker compose logs -f stress-tester"

# Piri with PostgreSQL database
up-piri-postgres:
	@if [ ! -d "generated/keys" ] || [ -z "$$(ls -A generated/keys 2>/dev/null)" ]; then \
		$(MAKE) init; \
	fi
	PIRI_DB_BACKEND=postgres $(DOCKER) compose --profile piri-postgres up -d
	@echo ""
	@echo "Services started with PostgreSQL database backend for piri."

# Piri with S3 blob storage (MinIO)
up-piri-s3:
	@if [ ! -d "generated/keys" ] || [ -z "$$(ls -A generated/keys 2>/dev/null)" ]; then \
		$(MAKE) init; \
	fi
	PIRI_BLOB_BACKEND=s3 $(DOCKER) compose --profile piri-s3 up -d
	@echo ""
	@echo "Services started with S3 (MinIO) blob storage for piri."
	@echo "MinIO Console: http://localhost:9003 (minioadmin/minioadmin)"

# Piri with both PostgreSQL and S3
up-piri-postgres-s3:
	@if [ ! -d "generated/keys" ] || [ -z "$$(ls -A generated/keys 2>/dev/null)" ]; then \
		$(MAKE) init; \
	fi
	PIRI_DB_BACKEND=postgres PIRI_BLOB_BACKEND=s3 $(DOCKER) compose --profile piri-postgres --profile piri-s3 up -d
	@echo ""
	@echo "Services started with PostgreSQL + S3 backends for piri."
	@echo "MinIO Console: http://localhost:9003 (minioadmin/minioadmin)"
