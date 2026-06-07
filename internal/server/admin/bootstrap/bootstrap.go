package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-idp/inlets/internal/server/admin/model"
	"github.com/go-zoox/gormx"
	_ "gorm.io/driver/sqlite"
)

// Init connects SQLite and runs migrations.
func Init(dbPath string) error {
	if dbPath == "" {
		return fmt.Errorf("admin database path is required")
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return fmt.Errorf("mkdir admin db dir: %w", err)
	}
	if err := gormx.LoadDB("sqlite", dbPath); err != nil {
		return fmt.Errorf("load admin db: %w", err)
	}
	if err := gormx.GetDB().AutoMigrate(model.MigrateModels()...); err != nil {
		return fmt.Errorf("migrate admin db: %w", err)
	}
	return nil
}
