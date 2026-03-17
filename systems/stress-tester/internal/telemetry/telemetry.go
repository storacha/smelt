package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

// Provider wraps the OTel meter provider and HTTP server
type Provider struct {
	meterProvider *metric.MeterProvider
	server        *http.Server
	port          int
}

// Config holds telemetry configuration
type Config struct {
	ServiceName    string
	PrometheusPort int
	OTLPEndpoint   string
}

// NewProvider creates a new telemetry provider
func NewProvider(cfg Config) (*Provider, error) {
	// Create resource with service info
	res, err := resource.New(context.Background(),
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion("0.1.0"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create Prometheus exporter for local /metrics endpoint
	promExporter, err := prometheus.New()
	if err != nil {
		return nil, fmt.Errorf("failed to create prometheus exporter: %w", err)
	}

	// Build meter provider options
	opts := []metric.Option{
		metric.WithResource(res),
		metric.WithReader(promExporter),
	}

	// Add OTLP exporter if endpoint is configured (push metrics to OTEL collector)
	if cfg.OTLPEndpoint != "" {
		slog.Info("configuring OTLP metrics export", "endpoint", cfg.OTLPEndpoint)
		otlpExporter, err := otlpmetrichttp.New(context.Background(),
			otlpmetrichttp.WithEndpoint(cfg.OTLPEndpoint),
			otlpmetrichttp.WithInsecure(),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
		}
		opts = append(opts, metric.WithReader(metric.NewPeriodicReader(otlpExporter)))
	}

	// Create meter provider
	meterProvider := metric.NewMeterProvider(opts...)

	// Set as global provider
	otel.SetMeterProvider(meterProvider)

	// Create HTTP server for /metrics endpoint
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.PrometheusPort),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	return &Provider{
		meterProvider: meterProvider,
		server:        server,
		port:          cfg.PrometheusPort,
	}, nil
}

// Start starts the metrics HTTP server
func (p *Provider) Start() error {
	go func() {
		if err := p.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			// Log error but don't crash
			fmt.Printf("metrics server error: %v\n", err)
		}
	}()
	return nil
}

// Stop shuts down the provider
func (p *Provider) Stop(ctx context.Context) error {
	if err := p.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown metrics server: %w", err)
	}
	if err := p.meterProvider.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown meter provider: %w", err)
	}
	return nil
}

// Port returns the port the metrics server is listening on
func (p *Provider) Port() int {
	return p.port
}

// MeterProvider returns the underlying meter provider
func (p *Provider) MeterProvider() *metric.MeterProvider {
	return p.meterProvider
}
