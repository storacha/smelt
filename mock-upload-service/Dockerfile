FROM golang:1.25-bookworm AS build

WORKDIR /go/src/mock-upload-service

# Copy go mod files first for caching
COPY go.mod go.sum* ./
RUN go mod download || true

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 go build -o /mock-upload-service ./cmd/main.go

# Final image with wget for health checks
FROM alpine:3.19

RUN apk --no-cache add ca-certificates wget curl

COPY --from=build /mock-upload-service /usr/bin/mock-upload-service

EXPOSE 8080

ENTRYPOINT ["/usr/bin/mock-upload-service", "serve"]
