package runner

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/storacha/smelt/systems/stress-tester/internal/config"
	"github.com/storacha/smelt/systems/stress-tester/internal/generator"
	"github.com/storacha/smelt/systems/stress-tester/internal/guppy"
	"github.com/storacha/smelt/systems/stress-tester/internal/store"
	"github.com/storacha/smelt/systems/stress-tester/internal/telemetry"
)

// UploadContinuousRunner runs continuous uploads
type UploadContinuousRunner struct {
	instanceID uint
	cfg        config.ContinuousUploadConfig
	store      store.Store
	guppy      guppy.Client
	metrics    *telemetry.Metrics
	generator  *generator.Generator
	email      string
	totalSize  int64
	interval   time.Duration
	duration   time.Duration

	status    atomic.Pointer[Status]
	startTime time.Time
	mu        sync.RWMutex
}

// NewUploadContinuousRunner creates a new continuous upload runner
func NewUploadContinuousRunner(
	instanceID uint,
	cfg config.ContinuousUploadConfig,
	store store.Store,
	guppy guppy.Client,
	metrics *telemetry.Metrics,
	gen *generator.Generator,
	email string,
) (*UploadContinuousRunner, error) {
	// Parse total size
	totalSize, err := generator.ParseByteSize(cfg.TotalSize)
	if err != nil {
		return nil, fmt.Errorf("invalid total_size: %w", err)
	}

	// Parse interval
	interval, err := time.ParseDuration(cfg.Interval)
	if err != nil {
		return nil, fmt.Errorf("invalid interval: %w", err)
	}

	// Parse duration (0 = forever)
	var duration time.Duration
	if cfg.Duration != "" && cfg.Duration != "0" {
		duration, err = time.ParseDuration(cfg.Duration)
		if err != nil {
			return nil, fmt.Errorf("invalid duration: %w", err)
		}
	}

	r := &UploadContinuousRunner{
		instanceID: instanceID,
		cfg:        cfg,
		store:      store,
		guppy:      guppy,
		metrics:    metrics,
		generator:  gen,
		email:      email,
		totalSize:  totalSize,
		interval:   interval,
		duration:   duration,
	}
	r.status.Store(&Status{
		Mode:  "upload_continuous",
		State: "idle",
	})
	return r, nil
}

// Run executes the continuous upload test
func (r *UploadContinuousRunner) Run(ctx context.Context) error {
	r.startTime = time.Now()
	r.updateStatus(func(s *Status) {
		s.State = "running"
		s.StartTime = r.startTime
	})

	// Apply duration limit if set
	if r.duration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.duration)
		defer cancel()
	}

	// Ensure guppy is logged in
	if err := r.guppy.Login(ctx, r.email); err != nil {
		slog.Warn("login failed, continuing anyway", "error", err)
	}

	slog.Info("starting continuous upload",
		"instance_id", r.instanceID,
		"interval", r.interval,
		"duration", r.duration,
		"total_size", r.cfg.TotalSize,
		"space_did", r.cfg.SpaceDID,
	)

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	// First upload immediately
	r.doUpload(ctx)

	for {
		select {
		case <-ctx.Done():
			r.logFinalStats()
			return nil
		case <-ticker.C:
			r.doUpload(ctx)
		}
	}
}

func (r *UploadContinuousRunner) doUpload(ctx context.Context) {
	// Check context before starting
	if ctx.Err() != nil {
		return
	}

	space := r.getSpace(ctx)
	if space == nil {
		slog.Warn("no space available, skipping upload")
		return
	}

	r.metrics.IncrementActiveUploads(ctx)
	defer r.metrics.DecrementActiveUploads(ctx)

	// Generate test data
	data, err := r.generator.Generate(r.totalSize)
	if err != nil {
		slog.Error("failed to generate test data", "error", err)
		r.updateStatus(func(s *Status) {
			s.UploadsFailed++
			s.LastError = err.Error()
		})
		r.metrics.RecordUpload(ctx, false, 0, 0, "continuous")
		return
	}

	startTime := time.Now()

	// Add source to space
	if err := r.guppy.AddSource(ctx, space.DID, data.Path); err != nil {
		slog.Error("failed to add source", "error", err, "space", space.DID)
		r.updateStatus(func(s *Status) {
			s.UploadsFailed++
			s.LastError = err.Error()
		})
		r.metrics.RecordUpload(ctx, false, time.Since(startTime), 0, "continuous")
		return
	}

	// Perform upload
	cids, err := r.guppy.Upload(ctx, space.DID)
	duration := time.Since(startTime)

	if err != nil {
		slog.Error("upload failed", "error", err, "space", space.DID)
		r.updateStatus(func(s *Status) {
			s.UploadsFailed++
			s.LastError = err.Error()
		})
		r.metrics.RecordUpload(ctx, false, duration, 0, "continuous")
		return
	}

	// Success
	r.metrics.RecordUpload(ctx, true, duration, data.SizeBytes, "continuous")
	r.updateStatus(func(s *Status) {
		s.UploadsTotal++
	})

	// Record in store
	if err := r.store.IncrementSpaceUploadedBytes(ctx, space.DID, data.SizeBytes); err != nil {
		slog.Error("failed to increment space bytes", "error", err)
	}

	if len(cids) > 0 {
		rootCID := cids[len(cids)-1]
		if _, err := r.store.RecordUpload(ctx, r.instanceID, space.DID, rootCID, data.Path, data.Hash, data.SizeBytes); err != nil {
			slog.Error("failed to record upload in db", "error", err, "cid", rootCID)
		}
	}

	slog.Debug("upload completed",
		"duration", duration,
		"size_bytes", data.SizeBytes,
		"space", space.DID,
	)

	r.maybeLogProgress()
}

func (r *UploadContinuousRunner) getSpace(ctx context.Context) *store.Space {
	if r.cfg.SpaceDID != "" {
		space, err := r.store.GetSpace(ctx, r.cfg.SpaceDID)
		if err != nil {
			slog.Error("failed to get specified space", "error", err, "space_did", r.cfg.SpaceDID)
			return nil
		}
		return space
	}
	space, err := r.store.GetRandomSpace(ctx, r.instanceID)
	if err != nil {
		slog.Error("failed to get random space", "error", err)
		return nil
	}
	return space
}

func (r *UploadContinuousRunner) maybeLogProgress() {
	status := r.status.Load()
	elapsed := time.Since(r.startTime)

	// Log every 10 uploads
	if status.UploadsTotal > 0 && status.UploadsTotal%10 == 0 {
		rate := float64(status.UploadsTotal) / elapsed.Seconds()
		slog.Info("progress",
			"uploads", status.UploadsTotal,
			"failed", status.UploadsFailed,
			"rate", fmt.Sprintf("%.2f/s", rate),
			"elapsed", elapsed.Round(time.Second),
		)
	}
}

func (r *UploadContinuousRunner) logFinalStats() {
	status := r.status.Load()
	elapsed := time.Since(r.startTime)

	var rate float64
	if elapsed.Seconds() > 0 {
		rate = float64(status.UploadsTotal) / elapsed.Seconds()
	}

	slog.Info("continuous upload finished",
		"total_uploads", status.UploadsTotal,
		"failed", status.UploadsFailed,
		"duration", elapsed.Round(time.Second),
		"rate", fmt.Sprintf("%.2f/s", rate),
	)
}

// GetStatus returns the current status
func (r *UploadContinuousRunner) GetStatus() *Status {
	return r.status.Load()
}

func (r *UploadContinuousRunner) updateStatus(fn func(*Status)) {
	r.mu.Lock()
	defer r.mu.Unlock()

	current := r.status.Load()
	newStatus := *current
	fn(&newStatus)
	r.status.Store(&newStatus)
}
