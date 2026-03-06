BINARY := stress
BUILD_DIR := ./bin

# Version info
VERSION ?= dev
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := -ldflags "-X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildTime=$(BUILD_TIME)"

.PHONY: build run clean test help

help:
	@echo "Stress Tester"
	@echo ""
	@echo "Usage:"
	@echo "  make build      Build the binary"
	@echo "  make run        Build and run with defaults"
	@echo "  make test       Run tests"
	@echo "  make clean      Remove build artifacts"
	@echo ""
	@echo "Run examples:"
	@echo "  make run ARGS='--mode burst --spaces 2 --uploads 5'"
	@echo "  make run ARGS='status'"

build:
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) ./cmd/stress

run: build
	$(BUILD_DIR)/$(BINARY) run $(ARGS)

test:
	go test -v ./...

clean:
	rm -rf $(BUILD_DIR)
	go clean
