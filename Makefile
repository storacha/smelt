SHELL := /bin/bash
DOCKER := $(shell which docker)

# Set YES=1 to skip confirmation prompts (e.g., make nuke YES=1)
YES ?= 0

.PHONY: help generate init up down restart clean nuke fresh logs pull build status guppy regen

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
	@echo "Development:"
	@echo "  make pull          Pull latest pre-built images"
	@echo "  make build         Build all Docker images"
	@echo "  make regen         Regenerate keys and proofs (requires restart)"
	@echo "  make logs          Follow all service logs"
	@echo "  make status        Show service status"
	@echo "  make shell-guppy   Open shell in guppy container"
	@echo ""
	@echo "Options:"
	@echo "  YES=1              Skip confirmation prompts (e.g., make nuke YES=1)"
	@echo ""
	@echo "Destructive commands (clean, nuke, fresh) require confirmation."
	@echo ""

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

# Start all services (runs init first if needed)
up:
	@if [ ! -d "generated/keys" ] || [ -z "$$(ls -A generated/keys 2>/dev/null)" ]; then \
		$(MAKE) init; \
	else \
		$(MAKE) generate; \
	fi
	$(DOCKER) compose up -d --remove-orphans
	@echo ""
	@echo "Services starting. Run 'make status' to check health."
	@echo "Run 'make logs' to follow logs."

# Stop all services (keeps volumes for quick restart)
down: generated/compose/piri.yml
	$(DOCKER) compose down --remove-orphans
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
clean: generated/compose/piri.yml
	$(call confirm,STOP all services and DELETE all volumes (Redis cache$(,) IPNI data$(,) etc.))
	@# Stop all services including those with profiles
	$(DOCKER) compose down -v --remove-orphans
	@# Also remove any dangling volumes from this project
	$(DOCKER) volume ls -q --filter "name=smelt_" | xargs -r $(DOCKER) volume rm 2>/dev/null || true
	@echo ""
	@echo "Services stopped and volumes removed."
	@echo "Keys and proofs preserved. Run 'make up' to restart."

# Remove EVERYTHING - volumes, keys, proofs, and built images
nuke: generated/compose/piri.yml
	$(call confirm,DELETE everything: containers$(,) volumes$(,) keys$(,) proofs$(,) AND Docker images)
	@echo "Removing all containers, volumes, keys, proofs, and images..."
	@# Stop all services including those with profiles
	$(DOCKER) compose down -v --remove-orphans --rmi local 2>/dev/null || true
	@# Also remove any dangling volumes from this project
	$(DOCKER) volume ls -q --filter "name=smelt_" | xargs -r $(DOCKER) volume rm 2>/dev/null || true
	rm -rf generated/keys generated/proofs generated/compose
	@echo ""
	@echo "Everything removed. Run 'make up' or 'make fresh' to start over."

# Complete fresh start - nuke everything, rebuild, and start
fresh: generated/compose/piri.yml
	$(call confirm,DELETE everything and rebuild from scratch)
	@echo "Removing all containers, volumes, keys, proofs, and images..."
	@# Stop all services including those with profiles
	$(DOCKER) compose down -v --remove-orphans --rmi local 2>/dev/null || true
	@# Also remove any dangling volumes from this project
	$(DOCKER) volume ls -q --filter "name=smelt_" | xargs -r $(DOCKER) volume rm 2>/dev/null || true
	rm -rf generated/keys generated/proofs generated/compose
	@echo ""
	@echo "Rebuilding and starting fresh..."
	$(MAKE) init
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
pull: generated/compose/piri.yml
	$(DOCKER) compose pull --ignore-pull-failures

# Build all images
build: generated/compose/piri.yml
	$(DOCKER) compose build

# Follow logs from all services
logs: generated/compose/piri.yml
	$(DOCKER) compose logs -f

# Show service status
status: generated/compose/piri.yml
	@$(DOCKER) compose ps
	@echo ""
	@$(DOCKER) compose ps --format "table {{.Name}}\t{{.Status}}" | grep -E "(healthy|unhealthy|starting)" || true

# Shell into guppy container
shell-guppy: generated/compose/piri.yml
	$(DOCKER) compose exec guppy bash

# Shell into piri-0 container
shell-piri: generated/compose/piri.yml
	$(DOCKER) compose exec piri-0 sh

# Shell into upload container
shell-upload:
	$(DOCKER) compose exec upload bash
