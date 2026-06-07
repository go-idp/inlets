package handler

import (
	"github.com/go-idp/inlets/internal/server/config"
	"gopkg.in/yaml.v3"
)

// parseConfigYAML parses a YAML config document into a FileConfig.
// It is a thin wrapper over yaml.Unmarshal to keep callers decoupled from
// the underlying parser.
func parseConfigYAML(raw []byte) (*config.FileConfig, error) {
	var cfg config.FileConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
