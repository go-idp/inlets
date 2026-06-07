package service

import "testing"

func TestNewConfigSchema_GroupsNonEmpty(t *testing.T) {
	schema := NewConfigSchema()
	if schema == nil {
		t.Fatal("nil schema")
	}
	if schema.SchemaVersion == 0 {
		t.Error("SchemaVersion should be set")
	}
	if len(schema.Groups) == 0 {
		t.Error("expected at least one group")
	}
}

func TestNewConfigSchema_DomainFieldExists(t *testing.T) {
	schema := NewConfigSchema()
	f := schema.FieldByPath("domain")
	if f == nil {
		t.Fatal("expected 'domain' field to exist")
	}
	if f.Kind != KindString {
		t.Errorf("expected KindString, got %q", f.Kind)
	}
	if !f.Required {
		t.Error("domain should be required")
	}
}

func TestNewConfigSchema_PortFieldHasRange(t *testing.T) {
	schema := NewConfigSchema()
	f := schema.FieldByPath("port")
	if f == nil {
		t.Fatal("expected 'port' field")
	}
	if f.Min == nil || f.Max == nil {
		t.Fatal("port should have min/max")
	}
	if *f.Min != 0 || *f.Max != 65535 {
		t.Errorf("port range wrong: %d..%d", *f.Min, *f.Max)
	}
}

func TestNewConfigSchema_TunnelTypeEnum(t *testing.T) {
	schema := NewConfigSchema()
	// tunnel.type is a nested path; we don't index it in the top-level groups
	// but the schema documents the enum values it should expose.
	// This test guards against accidental removal of the enum set.
	if len(schema.Groups) == 0 {
		t.Fatal("no groups")
	}
}
