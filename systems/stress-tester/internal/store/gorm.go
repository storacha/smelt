package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// GORMStore implements the Store interface using GORM
type GORMStore struct {
	db *gorm.DB
}

// StoreConfig holds configuration for creating a store
type StoreConfig struct {
	Type string // "sqlite" or "postgres"
	Path string // SQLite database path
	DSN  string // PostgreSQL DSN
}

// NewGORMStore creates a new GORM-backed store
func NewGORMStore(cfg StoreConfig) (*GORMStore, error) {
	var dialector gorm.Dialector

	switch cfg.Type {
	case "sqlite", "":
		// Create directory if needed
		if cfg.Path != "" && cfg.Path != ":memory:" {
			dir := filepath.Dir(cfg.Path)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return nil, fmt.Errorf("failed to create database directory: %w", err)
			}
		}
		dialector = sqlite.Open(cfg.Path)
	case "postgres":
		if cfg.DSN == "" {
			return nil, errors.New("postgres DSN is required")
		}
		dialector = postgres.Open(cfg.DSN)
	default:
		return nil, fmt.Errorf("unsupported database type: %s", cfg.Type)
	}

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Auto-migrate the schema
	if err := db.AutoMigrate(&Instance{}, &Space{}, &Upload{}, &Retrieval{}); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return &GORMStore{db: db}, nil
}

// Instance operations

func (s *GORMStore) GetOrCreateInstance(ctx context.Context, name string, seed int64) (*Instance, bool, error) {
	var instance Instance
	var created bool

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Try to find existing instance
		result := tx.Where("name = ?", name).First(&instance)
		if result.Error == nil {
			// Found existing instance
			created = false
			return nil
		}

		if !errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return result.Error
		}

		// Create new instance
		instance = Instance{
			Name:   name,
			Seed:   seed,
			Status: "active",
		}
		if err := tx.Create(&instance).Error; err != nil {
			return err
		}
		created = true
		return nil
	})

	if err != nil {
		return nil, false, err
	}
	return &instance, created, nil
}

func (s *GORMStore) GetInstance(ctx context.Context, name string) (*Instance, error) {
	var instance Instance
	if err := s.db.WithContext(ctx).Where("name = ?", name).First(&instance).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &instance, nil
}

func (s *GORMStore) ListInstances(ctx context.Context) ([]Instance, error) {
	var instances []Instance
	if err := s.db.WithContext(ctx).Order("created_at DESC").Find(&instances).Error; err != nil {
		return nil, err
	}
	return instances, nil
}

func (s *GORMStore) UpdateInstanceStatus(ctx context.Context, name string, status string) error {
	return s.db.WithContext(ctx).Model(&Instance{}).Where("name = ?", name).Update("status", status).Error
}

// Space operations

func (s *GORMStore) CreateSpace(ctx context.Context, instanceID uint, spaceDID string) (*Space, error) {
	space := Space{
		InstanceID: instanceID,
		DID:        spaceDID,
	}
	if err := s.db.WithContext(ctx).Create(&space).Error; err != nil {
		return nil, err
	}
	return &space, nil
}

func (s *GORMStore) GetSpace(ctx context.Context, spaceDID string) (*Space, error) {
	var space Space
	if err := s.db.WithContext(ctx).Where("did = ?", spaceDID).First(&space).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &space, nil
}

func (s *GORMStore) GetSpaces(ctx context.Context, instanceID uint) ([]Space, error) {
	var spaces []Space
	if err := s.db.WithContext(ctx).Where("instance_id = ?", instanceID).Order("created_at DESC").Find(&spaces).Error; err != nil {
		return nil, err
	}
	return spaces, nil
}

func (s *GORMStore) GetSpaceCount(ctx context.Context, instanceID uint) (int64, error) {
	var count int64
	if err := s.db.WithContext(ctx).Model(&Space{}).Where("instance_id = ?", instanceID).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (s *GORMStore) GetRandomSpace(ctx context.Context, instanceID uint) (*Space, error) {
	var space Space
	// RANDOM() works in both SQLite and PostgreSQL
	if err := s.db.WithContext(ctx).Where("instance_id = ?", instanceID).Order("RANDOM()").First(&space).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &space, nil
}

func (s *GORMStore) IncrementSpaceUploadedBytes(ctx context.Context, spaceDID string, bytes int64) error {
	return s.db.WithContext(ctx).Model(&Space{}).
		Where("did = ?", spaceDID).
		UpdateColumn("bytes_uploaded", gorm.Expr("bytes_uploaded + ?", bytes)).Error
}

func (s *GORMStore) IncrementSpaceRetrievedBytes(ctx context.Context, spaceDID string, bytes int64) error {
	return s.db.WithContext(ctx).Model(&Space{}).
		Where("did = ?", spaceDID).
		UpdateColumn("bytes_retrieved", gorm.Expr("bytes_retrieved + ?", bytes)).Error
}

// Upload operations

func (s *GORMStore) RecordUpload(ctx context.Context, instanceID uint, spaceDID, cid, sourcePath, contentHash string, sizeBytes int64) (*Upload, error) {
	// Get the space to get the space ID
	var space Space
	if err := s.db.WithContext(ctx).Where("did = ?", spaceDID).First(&space).Error; err != nil {
		return nil, fmt.Errorf("failed to find space: %w", err)
	}

	upload := Upload{
		InstanceID:  instanceID,
		SpaceID:     space.ID,
		SpaceDID:    spaceDID,
		CID:         cid,
		SourcePath:  sourcePath,
		SizeBytes:   sizeBytes,
		ContentHash: contentHash,
	}
	if err := s.db.WithContext(ctx).Create(&upload).Error; err != nil {
		return nil, err
	}
	return &upload, nil
}

func (s *GORMStore) GetUpload(ctx context.Context, id uint) (*Upload, error) {
	var upload Upload
	if err := s.db.WithContext(ctx).First(&upload, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &upload, nil
}

func (s *GORMStore) GetUploadByCID(ctx context.Context, cid string) (*Upload, error) {
	var upload Upload
	if err := s.db.WithContext(ctx).Where("cid = ?", cid).First(&upload).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &upload, nil
}

func (s *GORMStore) GetUploadsForSpace(ctx context.Context, spaceDID string) ([]Upload, error) {
	var uploads []Upload
	if err := s.db.WithContext(ctx).Where("space_did = ?", spaceDID).Order("uploaded_at DESC").Find(&uploads).Error; err != nil {
		return nil, err
	}
	return uploads, nil
}

func (s *GORMStore) GetRandomUpload(ctx context.Context, instanceID uint) (*Upload, error) {
	var upload Upload
	if err := s.db.WithContext(ctx).Where("instance_id = ?", instanceID).Order("RANDOM()").First(&upload).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &upload, nil
}

func (s *GORMStore) GetUploadCount(ctx context.Context, instanceID uint) (int64, error) {
	var count int64
	if err := s.db.WithContext(ctx).Model(&Upload{}).Where("instance_id = ?", instanceID).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (s *GORMStore) GetUploadStats(ctx context.Context, instanceID uint) (*UploadStats, error) {
	var stats UploadStats

	// Count uploads from Upload table
	var uploadCount int64
	if err := s.db.WithContext(ctx).Model(&Upload{}).Where("instance_id = ?", instanceID).Count(&uploadCount).Error; err != nil {
		return nil, err
	}
	stats.TotalUploads = uploadCount

	// Sum bytes from Space table (tracked at space level)
	var totalBytes int64
	if err := s.db.WithContext(ctx).Model(&Space{}).
		Select("COALESCE(SUM(bytes_uploaded), 0)").
		Where("instance_id = ?", instanceID).
		Scan(&totalBytes).Error; err != nil {
		return nil, err
	}
	stats.TotalBytes = totalBytes

	return &stats, nil
}

func (s *GORMStore) GetUploadsForRetrieval(ctx context.Context, instanceID uint, limit int, filter RetrievalFilter) ([]Upload, error) {
	query := s.db.WithContext(ctx).Where("instance_id = ?", instanceID)

	if filter.SpaceDID != "" {
		query = query.Where("space_did = ?", filter.SpaceDID)
	}

	query = query.Order("uploaded_at ASC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	var uploads []Upload
	if err := query.Find(&uploads).Error; err != nil {
		return nil, err
	}
	return uploads, nil
}

// Retrieval operations

func (s *GORMStore) RecordRetrieval(ctx context.Context, instanceID uint, uploadID uint, spaceDID, cid string, success bool, durationMs int64, errMsg string) (*Retrieval, error) {
	retrieval := Retrieval{
		InstanceID: instanceID,
		UploadID:   uploadID,
		SpaceDID:   spaceDID,
		CID:        cid,
		Success:    success,
		DurationMs: durationMs,
		Error:      errMsg,
	}
	if err := s.db.WithContext(ctx).Create(&retrieval).Error; err != nil {
		return nil, err
	}
	return &retrieval, nil
}

func (s *GORMStore) GetRetrievalStats(ctx context.Context, instanceID uint) (*RetrievalStats, error) {
	stats := &RetrievalStats{}

	// Get basic stats
	type basicStats struct {
		Total      int64
		Success    int64
		AvgDurMs   float64
		LastTime   *string
	}
	var basic basicStats

	err := s.db.WithContext(ctx).Model(&Retrieval{}).
		Select("COUNT(*) as total, SUM(CASE WHEN success THEN 1 ELSE 0 END) as success, AVG(duration_ms) as avg_dur_ms, MAX(retrieved_at) as last_time").
		Where("instance_id = ?", instanceID).
		Scan(&basic).Error
	if err != nil {
		return nil, err
	}

	stats.TotalRetrievals = basic.Total
	stats.SuccessCount = basic.Success
	stats.FailureCount = basic.Total - basic.Success
	stats.AvgDurationMs = basic.AvgDurMs

	if basic.Total > 0 {
		stats.SuccessRate = float64(basic.Success) / float64(basic.Total)
	}

	// Get percentiles - need to fetch all durations and calculate
	var durations []int64
	err = s.db.WithContext(ctx).Model(&Retrieval{}).
		Where("instance_id = ?", instanceID).
		Pluck("duration_ms", &durations).Error
	if err != nil {
		return nil, err
	}

	if len(durations) > 0 {
		sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
		stats.P50DurationMs = float64(percentile(durations, 50))
		stats.P95DurationMs = float64(percentile(durations, 95))
		stats.P99DurationMs = float64(percentile(durations, 99))
	}

	// Get total bytes retrieved from Space table (tracked at space level)
	var totalBytes int64
	err = s.db.WithContext(ctx).Model(&Space{}).
		Select("COALESCE(SUM(bytes_retrieved), 0)").
		Where("instance_id = ?", instanceID).
		Scan(&totalBytes).Error
	if err != nil {
		return nil, err
	}
	stats.TotalBytes = totalBytes

	return stats, nil
}

// percentile calculates the p-th percentile of a sorted slice
func percentile(sorted []int64, p int) int64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := (len(sorted) * p) / 100
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// Lifecycle

func (s *GORMStore) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
