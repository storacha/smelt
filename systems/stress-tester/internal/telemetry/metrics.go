package telemetry

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Metrics holds all metric instruments
type Metrics struct {
	meter metric.Meter

	// Counters
	SpacesCreated   metric.Int64Counter
	UploadsTotal    metric.Int64Counter
	UploadBytes     metric.Int64Counter
	RetrievalsTotal metric.Int64Counter
	VerifyTotal     metric.Int64Counter

	// Histograms
	UploadDuration    metric.Float64Histogram
	RetrievalDuration metric.Float64Histogram

	// Gauges (via UpDownCounter)
	ActiveUploads    metric.Int64UpDownCounter
	ActiveRetrievals metric.Int64UpDownCounter
}

// NewMetrics creates all metric instruments
func NewMetrics(provider *Provider) (*Metrics, error) {
	meter := provider.MeterProvider().Meter("stress-tester")

	spacesCreated, err := meter.Int64Counter("stress_spaces_created_total",
		metric.WithDescription("Total spaces created"),
		metric.WithUnit("{space}"),
	)
	if err != nil {
		return nil, err
	}

	uploadsTotal, err := meter.Int64Counter("stress_uploads_total",
		metric.WithDescription("Total upload attempts"),
		metric.WithUnit("{upload}"),
	)
	if err != nil {
		return nil, err
	}

	uploadBytes, err := meter.Int64Counter("stress_upload_bytes_total",
		metric.WithDescription("Total bytes uploaded"),
		metric.WithUnit("By"),
	)
	if err != nil {
		return nil, err
	}

	uploadDuration, err := meter.Float64Histogram("stress_upload_duration_seconds",
		metric.WithDescription("Upload duration in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.1, 0.5, 1, 2, 5, 10, 30, 60, 120, 300),
	)
	if err != nil {
		return nil, err
	}

	retrievalsTotal, err := meter.Int64Counter("stress_retrievals_total",
		metric.WithDescription("Total retrieval attempts"),
		metric.WithUnit("{retrieval}"),
	)
	if err != nil {
		return nil, err
	}

	retrievalDuration, err := meter.Float64Histogram("stress_retrieval_duration_seconds",
		metric.WithDescription("Retrieval duration in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.1, 0.5, 1, 2, 5, 10, 30, 60, 120, 300),
	)
	if err != nil {
		return nil, err
	}

	verifyTotal, err := meter.Int64Counter("stress_verify_total",
		metric.WithDescription("Total verification attempts"),
		metric.WithUnit("{verification}"),
	)
	if err != nil {
		return nil, err
	}

	activeUploads, err := meter.Int64UpDownCounter("stress_uploads_active",
		metric.WithDescription("Currently active uploads"),
		metric.WithUnit("{upload}"),
	)
	if err != nil {
		return nil, err
	}

	activeRetrievals, err := meter.Int64UpDownCounter("stress_retrievals_active",
		metric.WithDescription("Currently active retrievals"),
		metric.WithUnit("{retrieval}"),
	)
	if err != nil {
		return nil, err
	}

	return &Metrics{
		meter:             meter,
		SpacesCreated:     spacesCreated,
		UploadsTotal:      uploadsTotal,
		UploadBytes:       uploadBytes,
		UploadDuration:    uploadDuration,
		RetrievalsTotal:   retrievalsTotal,
		RetrievalDuration: retrievalDuration,
		VerifyTotal:       verifyTotal,
		ActiveUploads:     activeUploads,
		ActiveRetrievals:  activeRetrievals,
	}, nil
}

// RecordSpaceCreated increments the space creation counter
func (m *Metrics) RecordSpaceCreated(ctx context.Context) {
	m.SpacesCreated.Add(ctx, 1)
}

// RecordUpload records an upload attempt with timing and status
func (m *Metrics) RecordUpload(ctx context.Context, success bool, duration time.Duration, bytes int64, mode string) {
	status := "success"
	if !success {
		status = "failure"
	}

	attrs := metric.WithAttributes(
		attribute.String("status", status),
		attribute.String("mode", mode),
	)

	m.UploadsTotal.Add(ctx, 1, attrs)
	m.UploadDuration.Record(ctx, duration.Seconds(), attrs)

	if success && bytes > 0 {
		m.UploadBytes.Add(ctx, bytes, metric.WithAttributes(attribute.String("mode", mode)))
	}
}

// RecordRetrieval records a retrieval attempt with timing and status
func (m *Metrics) RecordRetrieval(ctx context.Context, success bool, duration time.Duration, mode string) {
	status := "success"
	if !success {
		status = "failure"
	}

	attrs := metric.WithAttributes(
		attribute.String("status", status),
		attribute.String("mode", mode),
	)

	m.RetrievalsTotal.Add(ctx, 1, attrs)
	m.RetrievalDuration.Record(ctx, duration.Seconds(), attrs)
}

// RecordVerification records a verification attempt
func (m *Metrics) RecordVerification(ctx context.Context, success bool) {
	status := "success"
	if !success {
		status = "failure"
	}

	m.VerifyTotal.Add(ctx, 1, metric.WithAttributes(attribute.String("status", status)))
}

// IncrementActiveUploads increments active upload count
func (m *Metrics) IncrementActiveUploads(ctx context.Context) {
	m.ActiveUploads.Add(ctx, 1)
}

// DecrementActiveUploads decrements active upload count
func (m *Metrics) DecrementActiveUploads(ctx context.Context) {
	m.ActiveUploads.Add(ctx, -1)
}

// IncrementActiveRetrievals increments active retrieval count
func (m *Metrics) IncrementActiveRetrievals(ctx context.Context) {
	m.ActiveRetrievals.Add(ctx, 1)
}

// DecrementActiveRetrievals decrements active retrieval count
func (m *Metrics) DecrementActiveRetrievals(ctx context.Context) {
	m.ActiveRetrievals.Add(ctx, -1)
}
