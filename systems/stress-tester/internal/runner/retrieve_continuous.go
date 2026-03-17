package runner

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/storacha/smelt/systems/stress-tester/internal/config"
	"github.com/storacha/smelt/systems/stress-tester/internal/guppy"
	"github.com/storacha/smelt/systems/stress-tester/internal/store"
	"github.com/storacha/smelt/systems/stress-tester/internal/telemetry"
)

// RetrieveContinuousRunner runs continuous retrievals
type RetrieveContinuousRunner struct {
	instanceID uint
	cfg        config.ContinuousRetrieveConfig
	store      store.Store
	guppy      guppy.Client
	metrics    *telemetry.Metrics
	email      string
	interval   time.Duration
	duration   time.Duration

	status    atomic.Pointer[Status]
	startTime time.Time
	mu        sync.RWMutex
}

// NewRetrieveContinuousRunner creates a new continuous retrieve runner
func NewRetrieveContinuousRunner(
	instanceID uint,
	cfg config.ContinuousRetrieveConfig,
	store store.Store,
	guppy guppy.Client,
	metrics *telemetry.Metrics,
	email string,
) (*RetrieveContinuousRunner, error) {
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

	r := &RetrieveContinuousRunner{
		instanceID: instanceID,
		cfg:        cfg,
		store:      store,
		guppy:      guppy,
		metrics:    metrics,
		email:      email,
		interval:   interval,
		duration:   duration,
	}
	r.status.Store(&Status{
		Mode:  "retrieve_continuous",
		State: "idle",
	})
	return r, nil
}

// Run executes the continuous retrieval test
func (r *RetrieveContinuousRunner) Run(ctx context.Context) error {
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

	slog.Info("starting continuous retrieval",
		"instance_id", r.instanceID,
		"interval", r.interval,
		"duration", r.duration,
		"space_did", r.cfg.SpaceDID,
	)

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	// First retrieval immediately
	r.doRetrieve(ctx)

	for {
		select {
		case <-ctx.Done():
			r.logFinalStats()
			return nil
		case <-ticker.C:
			r.doRetrieve(ctx)
		}
	}
}

func (r *RetrieveContinuousRunner) doRetrieve(ctx context.Context) {
	// Check context before starting
	if ctx.Err() != nil {
		return
	}

	upload := r.getUpload(ctx)
	if upload == nil {
		slog.Warn("no upload available for retrieval")
		return
	}

	r.metrics.IncrementActiveRetrievals(ctx)
	defer r.metrics.DecrementActiveRetrievals(ctx)

	// Create temp directory for retrieval
	destPath := fmt.Sprintf("/tmp/stress-retrieve-%d-%d", r.instanceID, upload.ID)

	startTime := time.Now()
	err := r.guppy.Retrieve(ctx, upload.SpaceDID, upload.CID, destPath)
	duration := time.Since(startTime)

	if err != nil {
		slog.Error("retrieval failed",
			"error", err,
			"cid", upload.CID,
			"space", upload.SpaceDID,
		)
		r.updateStatus(func(s *Status) {
			s.RetrievalsFailed++
			s.RetrievalsDone++
			s.LastError = err.Error()
		})
		r.metrics.RecordRetrieval(ctx, false, duration, "continuous")
		r.store.RecordRetrieval(ctx, r.instanceID, upload.ID, upload.SpaceDID, upload.CID, false, duration.Milliseconds(), err.Error())
		return
	}

	// Success
	r.updateStatus(func(s *Status) {
		s.RetrievalsDone++
	})
	r.metrics.RecordRetrieval(ctx, true, duration, "continuous")
	r.store.RecordRetrieval(ctx, r.instanceID, upload.ID, upload.SpaceDID, upload.CID, true, duration.Milliseconds(), "")

	// Increment space retrieved bytes
	if err := r.store.IncrementSpaceRetrievedBytes(ctx, upload.SpaceDID, upload.SizeBytes); err != nil {
		slog.Error("failed to increment retrieved bytes", "error", err)
	}

	slog.Debug("retrieval completed",
		"cid", upload.CID,
		"duration", duration,
	)

	// Cleanup temp directory
	os.RemoveAll(destPath)

	r.maybeLogProgress()
}

func (r *RetrieveContinuousRunner) getUpload(ctx context.Context) *store.Upload {
	if r.cfg.SpaceDID != "" {
		// Get random upload from specific space
		uploads, err := r.store.GetUploadsForRetrieval(ctx, r.instanceID, 1, store.RetrievalFilter{SpaceDID: r.cfg.SpaceDID})
		if err != nil {
			slog.Error("failed to get uploads for space", "error", err, "space_did", r.cfg.SpaceDID)
			return nil
		}
		if len(uploads) > 0 {
			return &uploads[0]
		}
		return nil
	}

	upload, err := r.store.GetRandomUpload(ctx, r.instanceID)
	if err != nil {
		slog.Error("failed to get random upload", "error", err)
		return nil
	}
	return upload
}

func (r *RetrieveContinuousRunner) maybeLogProgress() {
	status := r.status.Load()
	elapsed := time.Since(r.startTime)

	// Log every 10 retrievals
	if status.RetrievalsDone > 0 && status.RetrievalsDone%10 == 0 {
		rate := float64(status.RetrievalsDone) / elapsed.Seconds()
		successRate := float64(status.RetrievalsDone-status.RetrievalsFailed) / float64(status.RetrievalsDone) * 100
		slog.Info("progress",
			"retrievals", status.RetrievalsDone,
			"failed", status.RetrievalsFailed,
			"success_rate", fmt.Sprintf("%.1f%%", successRate),
			"rate", fmt.Sprintf("%.2f/s", rate),
			"elapsed", elapsed.Round(time.Second),
		)
	}
}

func (r *RetrieveContinuousRunner) logFinalStats() {
	status := r.status.Load()
	elapsed := time.Since(r.startTime)

	var rate float64
	if elapsed.Seconds() > 0 {
		rate = float64(status.RetrievalsDone) / elapsed.Seconds()
	}

	var successRate float64
	if status.RetrievalsDone > 0 {
		successRate = float64(status.RetrievalsDone-status.RetrievalsFailed) / float64(status.RetrievalsDone) * 100
	}

	slog.Info("continuous retrieval finished",
		"total_retrievals", status.RetrievalsDone,
		"failed", status.RetrievalsFailed,
		"success_rate", fmt.Sprintf("%.1f%%", successRate),
		"duration", elapsed.Round(time.Second),
		"rate", fmt.Sprintf("%.2f/s", rate),
	)
}

// GetStatus returns the current status
func (r *RetrieveContinuousRunner) GetStatus() *Status {
	return r.status.Load()
}

func (r *RetrieveContinuousRunner) updateStatus(fn func(*Status)) {
	r.mu.Lock()
	defer r.mu.Unlock()

	current := r.status.Load()
	newStatus := *current
	fn(&newStatus)
	r.status.Store(&newStatus)
}
