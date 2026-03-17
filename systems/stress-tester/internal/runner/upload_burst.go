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

// UploadBurstRunner runs a burst of uploads
type UploadBurstRunner struct {
	instanceID uint
	cfg        config.UploadBurstConfig
	store      store.Store
	guppy      guppy.Client
	metrics    *telemetry.Metrics
	generator  *generator.Generator
	email      string
	totalSize  int64 // parsed total size in bytes

	status atomic.Pointer[Status]
	mu     sync.RWMutex
}

// NewUploadBurstRunner creates a new upload burst runner
func NewUploadBurstRunner(
	instanceID uint,
	cfg config.UploadBurstConfig,
	store store.Store,
	guppy guppy.Client,
	metrics *telemetry.Metrics,
	gen *generator.Generator,
	email string,
) (*UploadBurstRunner, error) {
	// Parse total size
	totalSize, err := generator.ParseByteSize(cfg.TotalSize)
	if err != nil {
		return nil, fmt.Errorf("invalid total_size: %w", err)
	}

	r := &UploadBurstRunner{
		instanceID: instanceID,
		cfg:        cfg,
		store:      store,
		guppy:      guppy,
		metrics:    metrics,
		generator:  gen,
		email:      email,
		totalSize:  totalSize,
	}
	r.status.Store(&Status{
		Mode:  "upload_burst",
		State: "idle",
	})
	return r, nil
}

// Run executes the upload burst test
func (r *UploadBurstRunner) Run(ctx context.Context) error {
	startTime := time.Now()
	r.updateStatus(func(s *Status) {
		s.State = "running"
		s.StartTime = startTime
	})

	slog.Info("starting upload burst test",
		"instance_id", r.instanceID,
		"spaces", r.cfg.Spaces,
		"uploads_per_space", r.cfg.UploadsPerSpace,
		"concurrent", r.cfg.ConcurrentUploads,
		"total_size", r.cfg.TotalSize,
	)

	// Ensure guppy is logged in
	if err := r.guppy.Login(ctx, r.email); err != nil {
		slog.Warn("login failed, continuing anyway", "error", err)
	}

	// Create semaphore for concurrency control across spaces
	sem := make(chan struct{}, r.cfg.ConcurrentUploads)

	var wg sync.WaitGroup
	var errors []string
	var errorsMu sync.Mutex

	// Create spaces and perform uploads
	for i := 0; i < r.cfg.Spaces; i++ {
		if ctx.Err() != nil {
			break
		}

		wg.Add(1)
		sem <- struct{}{} // Acquire semaphore

		go func(spaceIndex int) {
			defer wg.Done()
			defer func() { <-sem }() // Release semaphore

			r.processSpace(ctx, spaceIndex, &errorsMu, &errors)
		}(i)
	}

	// Wait for all spaces to complete
	wg.Wait()

	endTime := time.Now()
	r.updateStatus(func(s *Status) {
		s.State = "completed"
		s.EndTime = &endTime
	})

	status := r.GetStatus()
	slog.Info("upload burst test completed",
		"duration", endTime.Sub(startTime),
		"spaces_created", status.SpacesCreated,
		"uploads_total", status.UploadsTotal,
		"uploads_failed", status.UploadsFailed,
	)

	// Log database summary
	r.logDatabaseSummary(ctx)

	if len(errors) > 0 {
		return fmt.Errorf("upload burst test completed with %d errors", len(errors))
	}

	return nil
}

func (r *UploadBurstRunner) logDatabaseSummary(ctx context.Context) {
	spaceCount, err := r.store.GetSpaceCount(ctx, r.instanceID)
	if err != nil {
		slog.Error("failed to get space count from db", "error", err)
		return
	}

	uploadCount, err := r.store.GetUploadCount(ctx, r.instanceID)
	if err != nil {
		slog.Error("failed to get upload count from db", "error", err)
		return
	}

	slog.Info("database summary",
		"instance_id", r.instanceID,
		"db_spaces", spaceCount,
		"db_uploads", uploadCount,
	)
}

// processSpace handles all uploads for a single space sequentially
func (r *UploadBurstRunner) processSpace(ctx context.Context, spaceIndex int, errorsMu *sync.Mutex, errors *[]string) {
	// Generate space
	provisionTo := guppy.EmailToDIDMailto(r.email)
	spaceDID, err := r.guppy.GenerateSpace(ctx, provisionTo)
	if err != nil {
		slog.Error("failed to generate space", "error", err)
		errorsMu.Lock()
		*errors = append(*errors, fmt.Sprintf("space generation failed: %v", err))
		errorsMu.Unlock()
		return
	}

	// Record space in store
	if space, err := r.store.CreateSpace(ctx, r.instanceID, spaceDID); err != nil {
		slog.Error("failed to record space in db", "error", err, "space", spaceDID)
	} else {
		slog.Debug("recorded space in db", "space_id", space.ID, "space_did", spaceDID)
	}

	r.updateStatus(func(s *Status) {
		s.SpacesCreated++
	})
	r.metrics.RecordSpaceCreated(ctx)

	slog.Info("created space", "space", spaceDID, "index", spaceIndex+1, "total", r.cfg.Spaces)

	// Process uploads for this space sequentially
	for j := 0; j < r.cfg.UploadsPerSpace; j++ {
		if ctx.Err() != nil {
			break
		}

		r.performUpload(ctx, spaceDID, spaceIndex, j, errorsMu, errors)
	}
}

func (r *UploadBurstRunner) performUpload(ctx context.Context, spaceDID string, spaceIndex, uploadIndex int, errorsMu *sync.Mutex, errors *[]string) {
	r.metrics.IncrementActiveUploads(ctx)
	defer r.metrics.DecrementActiveUploads(ctx)

	// Generate test data using the seeded generator
	data, err := r.generator.Generate(r.totalSize)
	if err != nil {
		slog.Error("failed to generate test data", "error", err)
		r.updateStatus(func(s *Status) {
			s.UploadsFailed++
			s.UploadsTotal++
			s.LastError = err.Error()
		})
		r.metrics.RecordUpload(ctx, false, 0, 0, "upload_burst")
		errorsMu.Lock()
		*errors = append(*errors, fmt.Sprintf("data generation failed: %v", err))
		errorsMu.Unlock()
		return
	}
	// NOTE: We intentionally do NOT cleanup temp data here.
	// Guppy remembers all sources added to a space.

	startTime := time.Now()

	// Add source to space
	if err := r.guppy.AddSource(ctx, spaceDID, data.Path); err != nil {
		slog.Error("failed to add source", "error", err, "space", spaceDID)
		r.updateStatus(func(s *Status) {
			s.UploadsFailed++
			s.UploadsTotal++
			s.LastError = err.Error()
		})
		r.metrics.RecordUpload(ctx, false, time.Since(startTime), 0, "upload_burst")
		errorsMu.Lock()
		*errors = append(*errors, fmt.Sprintf("add source failed: %v", err))
		errorsMu.Unlock()
		return
	}

	// Perform upload
	cids, err := r.guppy.Upload(ctx, spaceDID)
	duration := time.Since(startTime)

	if err != nil {
		slog.Error("upload failed", "error", err, "space", spaceDID)
		r.updateStatus(func(s *Status) {
			s.UploadsFailed++
			s.UploadsTotal++
			s.LastError = err.Error()
		})
		r.metrics.RecordUpload(ctx, false, duration, 0, "upload_burst")
		errorsMu.Lock()
		*errors = append(*errors, fmt.Sprintf("upload failed: %v", err))
		errorsMu.Unlock()
		return
	}

	// Record upload success
	r.updateStatus(func(s *Status) {
		s.UploadsTotal++
	})
	r.metrics.RecordUpload(ctx, true, duration, data.SizeBytes, "upload_burst")

	// Increment space bytes ONCE for this upload operation
	if err := r.store.IncrementSpaceUploadedBytes(ctx, spaceDID, data.SizeBytes); err != nil {
		slog.Error("failed to increment space bytes", "error", err)
	}

	// Record only the root CID (last one returned by guppy)
	if len(cids) > 0 {
		rootCID := cids[len(cids)-1]
		if upload, err := r.store.RecordUpload(ctx, r.instanceID, spaceDID, rootCID, data.Path, data.Hash, data.SizeBytes); err != nil {
			slog.Error("failed to record upload in db", "error", err, "cid", rootCID)
		} else {
			slog.Debug("recorded upload in db", "upload_id", upload.ID, "cid", rootCID)
		}
	}

	slog.Info("upload completed",
		"duration", duration,
		"size_bytes", data.SizeBytes,
		"file_count", data.FileCount,
		"dir_count", data.DirCount,
	)
}

// GetStatus returns the current status
func (r *UploadBurstRunner) GetStatus() *Status {
	return r.status.Load()
}

func (r *UploadBurstRunner) updateStatus(fn func(*Status)) {
	r.mu.Lock()
	defer r.mu.Unlock()

	current := r.status.Load()
	newStatus := *current
	fn(&newStatus)
	r.status.Store(&newStatus)
}
