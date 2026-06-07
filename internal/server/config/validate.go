package config

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ValidationError describes a single configuration problem with a
// machine-addressable path (dotted/indexed, e.g. "clients[2].clientSecret").
type ValidationError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

func (e ValidationError) Error() string {
	if e.Path == "" {
		return e.Message
	}
	return fmt.Sprintf("%s %s", e.Path, e.Message)
}

// validateWithDetails runs the same checks as Validate but returns a
// structured list of errors keyed by JSON-pointer-like paths. The function
// is pure (does not load files) and is safe to call from request handlers.
func ValidateWithDetails(cfg *FileConfig) []ValidationError {
	if cfg == nil {
		return []ValidationError{{Path: "", Message: "config is nil"}}
	}
	var errs []ValidationError

	if strings.TrimSpace(cfg.Domain) == "" {
		errs = append(errs, ValidationError{Path: "domain", Message: "is required"})
	}
	if cfg.Port < 0 || cfg.Port > 65535 {
		errs = append(errs, ValidationError{Path: "port", Message: "must be between 0 and 65535"})
	}
	if cfg.TCPPort < 0 || cfg.TCPPort > 65535 {
		errs = append(errs, ValidationError{Path: "tcpPort", Message: "must be between 0 and 65535"})
	}

	if len(cfg.Clients) == 0 {
		errs = append(errs, ValidationError{Path: "clients", Message: "is required and cannot be empty"})
	}

	seenClientIDs := make(map[string]int, len(cfg.Clients))
	for i, c := range cfg.Clients {
		base := fmt.Sprintf("clients[%d]", i)
		if strings.TrimSpace(c.ClientID) == "" {
			errs = append(errs, ValidationError{Path: base + ".clientId", Message: "is required"})
		} else {
			if prev, dup := seenClientIDs[c.ClientID]; dup {
				errs = append(errs, ValidationError{
					Path:    base + ".clientId",
					Message: fmt.Sprintf("duplicates clients[%d].clientId", prev),
				})
			}
			seenClientIDs[c.ClientID] = i
		}
		if strings.TrimSpace(c.ClientSecret) == "" {
			errs = append(errs, ValidationError{Path: base + ".clientSecret", Message: "is required"})
		}
		for j, t := range c.Tunnels {
			tpath := fmt.Sprintf("%s.tunnels[%d]", base, j)
			switch t.Type {
			case "http", "tcp":
				// ok
			default:
				errs = append(errs, ValidationError{Path: tpath + ".type", Message: `must be "http" or "tcp"`})
			}
			if strings.TrimSpace(t.Upstream) == "" {
				errs = append(errs, ValidationError{Path: tpath + ".upstream", Message: "is required"})
			}
		}
	}

	if cfg.PublicHTTPNoAuth != nil {
		if v := strings.TrimSpace(cfg.PublicHTTPNoAuth.Timeout); v != "" {
			if _, err := time.ParseDuration(v); err != nil {
				errs = append(errs, ValidationError{Path: "publicHTTPNoAuth.timeout", Message: "invalid duration: " + err.Error()})
			}
		}
		if v := strings.TrimSpace(cfg.PublicHTTPNoAuth.WarnLead); v != "" {
			if _, err := time.ParseDuration(v); err != nil {
				errs = append(errs, ValidationError{Path: "publicHTTPNoAuth.warnLead", Message: "invalid duration: " + err.Error()})
			}
		}
	}

	if cfg.Admin != nil && cfg.Admin.Enabled {
		if _, err := ResolveAdmin(cfg, ""); err != nil {
			errs = append(errs, ValidationError{Path: "admin", Message: err.Error()})
		}
	}

	return errs
}

// Validate returns a single error (errors.Join) or nil.
// Kept for callers that only need a yes/no answer.
func Validate(cfg *FileConfig) error {
	details := ValidateWithDetails(cfg)
	if len(details) == 0 {
		return nil
	}
	wrapped := make([]error, 0, len(details))
	for _, d := range details {
		wrapped = append(wrapped, d)
	}
	return errors.Join(wrapped...)
}
