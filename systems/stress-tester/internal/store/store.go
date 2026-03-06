package store

import (
	"context"
	"time"
)

// RetrievalFilter specifies filters for querying uploads for retrieval
type RetrievalFilter struct {
	SpaceDID string // empty = all spaces
}

// UploadStats holds aggregate upload statistics
type UploadStats struct {
	TotalUploads int64
	TotalBytes   int64
}

// RetrievalStats holds aggregate retrieval statistics
type RetrievalStats struct {
	TotalRetrievals   int64
	SuccessCount      int64
	FailureCount      int64
	TotalBytes        int64 // Sum of bytes for successful retrievals
	AvgDurationMs     float64
	P50DurationMs     float64
	P95DurationMs     float64
	P99DurationMs     float64
	SuccessRate       float64
	LastRetrievalTime *time.Time
}

// Store defines the interface for persistence
type Store interface {
	// Instance operations
	GetOrCreateInstance(ctx context.Context, name string, seed int64) (*Instance, bool, error) // returns (instance, created, error)
	GetInstance(ctx context.Context, name string) (*Instance, error)
	ListInstances(ctx context.Context) ([]Instance, error)
	UpdateInstanceStatus(ctx context.Context, name string, status string) error

	// Spaces (scoped to instance)
	CreateSpace(ctx context.Context, instanceID uint, spaceDID string) (*Space, error)
	GetSpace(ctx context.Context, spaceDID string) (*Space, error)
	GetSpaces(ctx context.Context, instanceID uint) ([]Space, error)
	GetSpaceCount(ctx context.Context, instanceID uint) (int64, error)
	GetRandomSpace(ctx context.Context, instanceID uint) (*Space, error)
	IncrementSpaceUploadedBytes(ctx context.Context, spaceDID string, bytes int64) error
	IncrementSpaceRetrievedBytes(ctx context.Context, spaceDID string, bytes int64) error

	// Uploads (scoped to instance)
	RecordUpload(ctx context.Context, instanceID uint, spaceDID, cid, sourcePath, contentHash string, sizeBytes int64) (*Upload, error)
	GetUpload(ctx context.Context, id uint) (*Upload, error)
	GetUploadByCID(ctx context.Context, cid string) (*Upload, error)
	GetUploadsForSpace(ctx context.Context, spaceDID string) ([]Upload, error)
	GetRandomUpload(ctx context.Context, instanceID uint) (*Upload, error)
	GetUploadCount(ctx context.Context, instanceID uint) (int64, error)
	GetUploadStats(ctx context.Context, instanceID uint) (*UploadStats, error)
	GetUploadsForRetrieval(ctx context.Context, instanceID uint, limit int, filter RetrievalFilter) ([]Upload, error)

	// Retrievals (scoped to instance)
	RecordRetrieval(ctx context.Context, instanceID uint, uploadID uint, spaceDID, cid string, success bool, durationMs int64, errMsg string) (*Retrieval, error)
	GetRetrievalStats(ctx context.Context, instanceID uint) (*RetrievalStats, error)

	// Lifecycle
	Close() error
}
