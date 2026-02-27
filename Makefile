SHELL := /bin/bash
DOCKER := /usr/bin/docker

# Set YES=1 to skip confirmation prompts (e.g., make nuke YES=1)
YES ?= 0

.PHONY: help init up down restart clean nuke fresh logs build status guppy regen

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
	@echo "  make build     Build all Docker images"
	@echo "  make regen     Regenerate keys and proofs (requires restart)"
	@echo "  make logs      Follow all service logs"
	@echo "  make status    Show service status"
	@echo "  make guppy     Open shell in guppy container"
	@echo ""
	@echo "Destructive commands (clean, nuke, fresh) require confirmation."
	@echo "Skip with: make <command> YES=1"
	@echo ""

# Initialize the environment (generate keys, proofs, create network)
init:
	@./scripts/init.sh

# Start all services (runs init first if needed)
up:
	@if [ ! -d "generated/keys" ] || [ -z "$$(ls -A generated/keys 2>/dev/null)" ]; then \
		$(MAKE) init; \
	fi
	$(DOCKER) compose up -d
	@echo ""
	@echo "Services starting. Run 'make status' to check health."
	@echo "Run 'make logs' to follow logs."

# Stop all services (keeps volumes for quick restart)
down:
	$(DOCKER) compose down
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
	$(DOCKER) compose down -v --remove-orphans
	@echo ""
	@echo "Services stopped and volumes removed."
	@echo "Keys and proofs preserved. Run 'make up' to restart."

# Remove EVERYTHING - volumes, keys, proofs, and built images
nuke:
	$(call confirm,DELETE everything: containers$(,) volumes$(,) keys$(,) proofs$(,) AND Docker images)
	@echo "Removing all containers, volumes, keys, proofs, and images..."
	$(DOCKER) compose down -v --remove-orphans --rmi local 2>/dev/null || true
	rm -rf generated/keys generated/proofs
	@echo ""
	@echo "Everything removed. Run 'make up' or 'make fresh' to start over."

# Complete fresh start - nuke everything, rebuild, and start
fresh:
	$(call confirm,DELETE everything and rebuild from scratch)
	@echo "Removing all containers, volumes, keys, proofs, and images..."
	$(DOCKER) compose down -v --remove-orphans --rmi local 2>/dev/null || true
	rm -rf generated/keys generated/proofs
	@echo ""
	@echo "Rebuilding and starting fresh..."
	$(MAKE) init
	$(DOCKER) compose build
	$(DOCKER) compose up -d
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

# Build all images
build:
	$(DOCKER) compose build

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
	$(DOCKER) compose exec piri bash
