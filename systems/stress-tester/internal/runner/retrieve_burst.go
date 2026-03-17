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

// RetrieveBurstRunner runs a burst of retrievals
type RetrieveBurstRunner struct {
	instanceID uint
	cfg        config.RetrieveBurstConfig
	store      store.Store
	guppy      guppy.Client
	metrics    *telemetry.Metrics
	email      string

	status atomic.Pointer[Status]
	mu     sync.RWMutex
}

// NewRetrieveBurstRunner creates a new retrieve burst runner
func NewRetrieveBurstRunner(
	instanceID uint,
	cfg config.RetrieveBurstConfig,
	store store.Store,
	guppy guppy.Client,
	metrics *telemetry.Metrics,
	email string,
) *RetrieveBurstRunner {
	r := &RetrieveBurstRunner{
		instanceID: instanceID,
		cfg:        cfg,
		store:      store,
		guppy:      guppy,
		metrics:    metrics,
		email:      email,
	}
	r.status.Store(&Status{
		Mode:  "retrieve_burst",
		State: "idle",
	})
	return r
}

// Run executes the retrieve burst test
func (r *RetrieveBurstRunner) Run(ctx context.Context) error {
	startTime := time.Now()
	r.updateStatus(func(s *Status) {
		s.State = "running"
		s.StartTime = startTime
	})

	slog.Info("starting retrieve burst test",
		"instance_id", r.instanceID,
		"concurrent", r.cfg.ConcurrentRetrievals,
		"limit", r.cfg.Limit,
		"space_did", r.cfg.SpaceDID,
	)

	// Ensure guppy is logged in
	if err := r.guppy.Login(ctx, r.email); err != nil {
		slog.Warn("login failed, continuing anyway", "error", err)
	}

	// Get uploads for retrieval
	filter := store.RetrievalFilter{
		SpaceDID: r.cfg.SpaceDID,
	}
	uploads, err := r.store.GetUploadsForRetrieval(ctx, r.instanceID, r.cfg.Limit, filter)
	if err != nil {
		return fmt.Errorf("failed to get uploads for retrieval: %w", err)
	}

	if len(uploads) == 0 {
		slog.Info("no uploads found for retrieval")
		return nil
	}

	slog.Info("found uploads for retrieval", "count", len(uploads))

	// Create semaphore for concurrency control
	sem := make(chan struct{}, r.cfg.ConcurrentRetrievals)

	var wg sync.WaitGroup
	var errors []string
	var errorsMu sync.Mutex

	for _, upload := range uploads {
		if ctx.Err() != nil {
			break
		}

		wg.Add(1)
		sem <- struct{}{} // Acquire semaphore

		go func(u store.Upload) {
			defer wg.Done()
			defer func() { <-sem }() // Release semaphore

			r.performRetrieval(ctx, &u, &errorsMu, &errors)
		}(upload)
	}

	// Wait for all retrievals to complete
	wg.Wait()

	endTime := time.Now()
	r.updateStatus(func(s *Status) {
		s.State = "completed"
		s.EndTime = &endTime
	})

	status := r.GetStatus()
	slog.Info("retrieve burst test completed",
		"duration", endTime.Sub(startTime),
		"retrievals_done", status.RetrievalsDone,
		"retrievals_failed", status.RetrievalsFailed,
	)

	// Log database summary
	r.logDatabaseSummary(ctx)

	if len(errors) > 0 {
		return fmt.Errorf("retrieve burst test completed with %d errors", len(errors))
	}

	return nil
}

func (r *RetrieveBurstRunner) logDatabaseSummary(ctx context.Context) {
	stats, err := r.store.GetRetrievalStats(ctx, r.instanceID)
	if err != nil {
		slog.Error("failed to get retrieval stats from db", "error", err)
		return
	}

	slog.Info("database summary",
		"instance_id", r.instanceID,
		"db_retrievals", stats.TotalRetrievals,
		"db_retrieval_success", stats.SuccessCount,
		"db_retrieval_failed", stats.FailureCount,
		"db_success_rate", fmt.Sprintf("%.1f%%", stats.SuccessRate*100),
	)
}

func (r *RetrieveBurstRunner) performRetrieval(ctx context.Context, upload *store.Upload, errorsMu *sync.Mutex, errors *[]string) {
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
		r.metrics.RecordRetrieval(ctx, false, duration, "retrieve_burst")
		r.store.RecordRetrieval(ctx, r.instanceID, upload.ID, upload.SpaceDID, upload.CID, false, duration.Milliseconds(), err.Error())

		errorsMu.Lock()
		*errors = append(*errors, fmt.Sprintf("retrieval failed for %s: %v", upload.CID, err))
		errorsMu.Unlock()
		return
	}

	// Retrieval succeeded
	r.updateStatus(func(s *Status) {
		s.RetrievalsDone++
	})
	r.metrics.RecordRetrieval(ctx, true, duration, "retrieve_burst")
	r.store.RecordRetrieval(ctx, r.instanceID, upload.ID, upload.SpaceDID, upload.CID, true, duration.Milliseconds(), "")

	// Increment space retrieved bytes
	if err := r.store.IncrementSpaceRetrievedBytes(ctx, upload.SpaceDID, upload.SizeBytes); err != nil {
		slog.Error("failed to increment retrieved bytes", "error", err)
	}

	slog.Info("retrieval completed",
		"cid", upload.CID,
		"duration", duration,
	)

	// Cleanup temp directory
	os.RemoveAll(destPath)
}

// GetStatus returns the current status
func (r *RetrieveBurstRunner) GetStatus() *Status {
	return r.status.Load()
}

func (r *RetrieveBurstRunner) updateStatus(fn func(*Status)) {
	r.mu.Lock()
	defer r.mu.Unlock()

	current := r.status.Load()
	newStatus := *current
	fn(&newStatus)
	r.status.Store(&newStatus)
}
