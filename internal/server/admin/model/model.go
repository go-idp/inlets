package model

import "time"

// AuditLog records admin actions.
type AuditLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Action    string    `gorm:"size:64;index" json:"action"`
	Summary   string    `gorm:"size:512" json:"summary"`
	Actor     string    `gorm:"size:128" json:"actor"`
	ClientIP  string    `gorm:"size:64" json:"clientIp"`
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

func MigrateModels() []any {
	return []any{&AuditLog{}, &MetricSnapshot{}}
}
