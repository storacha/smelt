package runner

import (
	"context"
	"time"
)

// Runner defines the interface for stress test runners
type Runner interface {
	// Run executes the stress test
	Run(ctx context.Context) error

	// GetStatus returns the current status
	GetStatus() *Status
}

// Status represents the current state of a runner
type Status struct {
	Mode             string
	State            string // "idle", "running", "completed", "failed"
	StartTime        time.Time
	EndTime          *time.Time
	SpacesCreated    int64
	UploadsTotal     int64
	UploadsFailed    int64
	RetrievalsDone   int64
	RetrievalsFailed int64
	LastError        string
}

// Result represents the final result of a stress test run
type Result struct {
	Success             bool
	Duration            time.Duration
	SpacesCreated       int64
	UploadsAttempted    int64
	UploadsSucceeded    int64
	UploadsFailed       int64
	RetrievalsAttempted int64
	RetrievalsSucceeded int64
	RetrievalsFailed    int64
	Errors              []string
}
