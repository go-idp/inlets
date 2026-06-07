package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Load reads and parses a YAML config file.
func Load(path string) (*FileConfig, error) {
	if path == "" {
		return nil, fmt.Errorf("config path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg FileConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}

// LoadRaw returns the raw YAML bytes.
func LoadRaw(path string) ([]byte, error) {
	if path == "" {
		return nil, fmt.Errorf("config path is required")
	}
	return os.ReadFile(path)
}
