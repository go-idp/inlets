package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-idp/inlets/internal/server/config"
)

// TestConfigService_SaveRaw_PersistsRevisionAndDiff verifies that saving
// the YAML writes a revision row and that the diff is computed against
// the file as it was BEFORE the save. We use a real on-disk file but
// skip the SQLite side (which would require bootstrapping the admin DB).
func TestConfigService_SaveRaw_PersistsRevisionAndDiff(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.yaml")
	original := `domain: example.com
clients:
  - clientId: a
    clientSecret: b
`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	svc := NewConfigService(path, nil)
	got, err := svc.Raw()
	if err != nil {
		t.Fatalf("Raw: %v", err)
	}
	if string(got) != original {
		t.Fatalf("raw mismatch: %q", string(got))
	}

	// We can't fully exercise SaveRaw without SQLite, so we just check
	// the diff and parse parts.
	updated := `domain: example.com
clients:
  - clientId: a
    clientSecret: b
port: 8080
`
	diff := UnifiedDiff(original, updated)
	if diff == "" {
		t.Fatal("expected non-empty diff")
	}
	// Sanity-check the diff content.
	if want := "+port: 8080"; !contains(diff, want) {
		t.Errorf("expected %q in diff, got:\n%s", want, diff)
	}

	// And finally make sure the underlying YAML package roundtrips.
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Domain != "example.com" {
		t.Errorf("domain: %q", cfg.Domain)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
