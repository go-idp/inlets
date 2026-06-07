package service

import (
	"github.com/go-idp/inlets/internal/server/admin/model"
	"github.com/go-zoox/gormx"
)

// Audit records admin actions in SQLite.
type Audit struct{}

func NewAudit() *Audit {
	return &Audit{}
}

func (a *Audit) Record(action, summary, actor, clientIP string) (*model.AuditLog, error) {
	row := &model.AuditLog{
		Action:   action,
		Summary:  summary,
		Actor:    actor,
		ClientIP: clientIP,
	}
	if _, err := gormx.Create(row); err != nil {
		return nil, err
	}
	return row, nil
}

func (a *Audit) List(limit int) ([]*model.AuditLog, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	var rows []*model.AuditLog
	err := gormx.GetDB().Order("created_at DESC").Limit(limit).Find(&rows).Error
	return rows, err
}
