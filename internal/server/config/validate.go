package config

import (
	"fmt"
	"strings"
)

// Validate checks required fields for a server config document.
func Validate(cfg *FileConfig) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	if strings.TrimSpace(cfg.Domain) == "" {
		return fmt.Errorf("domain is required")
	}
	if len(cfg.Clients) == 0 {
		return fmt.Errorf("clients configuration is required and cannot be empty")
	}
	for i, c := range cfg.Clients {
		if strings.TrimSpace(c.ClientID) == "" {
			return fmt.Errorf("clients[%d].clientId is required", i)
		}
		if strings.TrimSpace(c.ClientSecret) == "" {
			return fmt.Errorf("clients[%d].clientSecret is required", i)
		}
	}
	if cfg.Admin != nil && cfg.Admin.Enabled {
		if _, err := ResolveAdmin(cfg, ""); err != nil {
			return err
		}
	}
	return nil
}
