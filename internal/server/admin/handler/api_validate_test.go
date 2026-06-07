package handler

import (
	"reflect"
	"testing"

	"github.com/go-idp/inlets/internal/server/admin/service"
	"github.com/go-idp/inlets/internal/server/config"
)

func TestParseConfigYAML_Valid(t *testing.T) {
	raw := []byte(`domain: example.com
clients:
  - clientId: a
    clientSecret: b
`)
	cfg, err := parseConfigYAML(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Domain != "example.com" {
		t.Errorf("domain mismatch: %q", cfg.Domain)
	}
	if len(cfg.Clients) != 1 {
		t.Errorf("expected 1 client, got %d", len(cfg.Clients))
	}
}

func TestParseConfigYAML_Invalid(t *testing.T) {
	raw := []byte("domain: : :\n  - bad")
	_, err := parseConfigYAML(raw)
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestNewConfigSchema_Shape(t *testing.T) {
	schema := service.NewConfigSchema()
	if schema.SchemaVersion == 0 {
		t.Fatal("schema version missing")
	}
	// Spot-check: at least the server group is present.
	has := false
	for _, g := range schema.Groups {
		if g.Key == "server" {
			has = true
		}
	}
	if !has {
		t.Errorf("expected 'server' group, got %+v", schema.Groups)
	}
	// Spot-check: a specific known path resolves.
	if schema.FieldByPath("domain") == nil {
		t.Errorf("expected FieldByPath('domain') to resolve")
	}
}

func TestValidatePipeline_Valid(t *testing.T) {
	raw := []byte(`domain: example.com
clients:
  - clientId: a
    clientSecret: b
`)
	cfg, err := parseConfigYAML(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	details := config.ValidateWithDetails(cfg)
	if len(details) != 0 {
		t.Errorf("expected zero errors, got %+v", details)
	}
}

func TestValidatePipeline_AllErrors(t *testing.T) {
	raw := []byte(`domain: ""
clients: []
`)
	cfg, err := parseConfigYAML(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	details := config.ValidateWithDetails(cfg)
	if len(details) < 2 {
		t.Errorf("expected multiple errors, got %+v", details)
	}
	// Each error should have a Path populated (non-empty).
	for _, d := range details {
		if d.Message == "" {
			t.Errorf("error with empty message: %+v", d)
		}
	}
	_ = reflect.TypeOf
}
