FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build with version info
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

RUN CGO_ENABLED=0 go build \
    -ldflags "-X main.Version=${VERSION} -X main.Commit=${COMMIT} -X main.BuildTime=${BUILD_TIME}" \
    -o /stress ./cmd/stress

# Runtime image - use Debian to match guppy's glibc requirements
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    wget \
    bash \
    vim \
    curl \
    less \
    procps \
    && rm -rf /var/lib/apt/lists/*

# Set bash as default shell
SHELL ["/bin/bash", "-c"]

# Copy stress binary
COPY --from=builder /stress /usr/local/bin/stress

# Copy guppy binary from guppy image
COPY --from=forreststoracha/guppy:dev /usr/bin/guppy /usr/local/bin/guppy

# Copy default config (container version with correct paths)
COPY config/container.yaml /etc/stress/config.yaml

# Create data directory
RUN mkdir -p /data

# Set environment defaults
ENV STRESS_STORE_PATH=/data/stress.db
ENV STRESS_TELEMETRY_PROMETHEUS_PORT=9091

EXPOSE 9091

WORKDIR /data

# Default: bash shell for interactive use
# Override with: docker run ... stress run
CMD ["/bin/bash"]
