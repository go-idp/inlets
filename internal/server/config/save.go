package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// SaveAtomic writes cfg to path using a temp file and rename.
func SaveAtomic(path string, cfg *FileConfig) error {
	if path == "" {
		return fmt.Errorf("config path is required")
	}
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	if err := Validate(cfg); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir config dir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename config: %w", err)
	}
	return nil
}

// SaveRawAtomic writes raw YAML to path.
func SaveRawAtomic(path string, raw []byte) error {
	if path == "" {
		return fmt.Errorf("config path is required")
	}
	cfg, err := parseDocument(raw)
	if err != nil {
		return err
	}
	if err := Validate(cfg); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir config dir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename config: %w", err)
	}
	return nil
}

func parseDocument(raw []byte) (*FileConfig, error) {
	var cfg FileConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}
