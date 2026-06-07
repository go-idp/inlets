package model

import "time"

// AuditLog records admin actions.
type AuditLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Action    string    `gorm:"size:64;index" json:"action"`
	Summary   string    `gorm:"size:512" json:"summary"`
	Actor     string    `gorm:"size:128" json:"actor"`
	ClientIP  string    `gorm:"size:64" json:"clientIp"`
	Diff      string    `gorm:"type:text" json:"diff,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// MetricSnapshot stores periodic traffic aggregates.
type MetricSnapshot struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	UploadBytes   int64     `json:"uploadBytes"`
	DownloadBytes int64     `json:"downloadBytes"`
	Requests      int64     `json:"requests"`
	Connections   int64     `json:"connections"`
	SessionCount  int       `json:"sessionCount"`
	CreatedAt     time.Time `gorm:"index" json:"createdAt"`
}

// ConfigRevision is a snapshot of the YAML config saved on each PUT.
// Restoring a revision creates a new revision; history is append-only.
type ConfigRevision struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time `gorm:"index" json:"createdAt"`
	Actor     string    `gorm:"size:128" json:"actor"`
	ClientIP  string    `gorm:"size:64" json:"clientIp"`
	Summary   string    `gorm:"size:512" json:"summary"`
	YAML      string    `gorm:"type:text" json:"yaml"`
	BytesSize int       `json:"bytesSize"`
}

func MigrateModels() []any {
	return []any{&AuditLog{}, &MetricSnapshot{}, &ConfigRevision{}}
}
