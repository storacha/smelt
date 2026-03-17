package store

import "time"

// Instance represents a stress test instance for namespacing/bounding concurrent runs
type Instance struct {
	ID        uint      `gorm:"primaryKey;autoIncrement"`
	Name      string    `gorm:"uniqueIndex;not null"`
	Seed      int64     `gorm:"not null"`
	Status    string    `gorm:"default:'active'"` // "active", "completed"
	CreatedAt time.Time `gorm:"autoCreateTime"`
}

// Space represents a storage space within an instance
type Space struct {
	ID             uint      `gorm:"primaryKey;autoIncrement"`
	InstanceID     uint      `gorm:"not null;index"`
	Instance       Instance  `gorm:"foreignKey:InstanceID"`
	DID            string    `gorm:"column:did;uniqueIndex;not null"`
	BytesUploaded  int64     `gorm:"default:0"`
	BytesRetrieved int64     `gorm:"default:0"`
	CreatedAt      time.Time `gorm:"autoCreateTime"`
}

// Upload represents an upload record within an instance
type Upload struct {
	ID          uint      `gorm:"primaryKey;autoIncrement"`
	InstanceID  uint      `gorm:"not null;index"` // denormalized for query efficiency
	Instance    Instance  `gorm:"foreignKey:InstanceID"`
	SpaceID     uint      `gorm:"not null;index"`
	Space       Space     `gorm:"foreignKey:SpaceID"`
	SpaceDID    string    `gorm:"not null;index"`
	CID         string    `gorm:"not null;index"`
	SourcePath  string    `gorm:"not null"`
	SizeBytes   int64     `gorm:"not null"`
	ContentHash string    `gorm:"not null"`
	UploadedAt  time.Time `gorm:"autoCreateTime"`
}

// Retrieval represents a retrieval record within an instance
type Retrieval struct {
	ID          uint      `gorm:"primaryKey;autoIncrement"`
	InstanceID  uint      `gorm:"not null;index"` // denormalized for query efficiency
	Instance    Instance  `gorm:"foreignKey:InstanceID"`
	UploadID    uint      `gorm:"not null;index"`
	Upload      Upload    `gorm:"foreignKey:UploadID"`
	SpaceDID    string    `gorm:"not null"`
	CID         string    `gorm:"not null"`
	Success     bool      `gorm:"not null"`
	DurationMs  int64     `gorm:"not null"`
	RetrievedAt time.Time `gorm:"autoCreateTime"`
	Error       string
}
