package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileAtomicRenameFallback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.yaml")
	if err := os.WriteFile(path, []byte("domain: old\nport: 80\n"), 0o600); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	f, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		t.Fatalf("open config: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	next := []byte("domain: new\nport: 8080\nclients:\n  - clientId: a\n    clientSecret: b\n")
	if err := writeFileAtomic(path, next, 0o600); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if string(got) != string(next) {
		t.Fatalf("got %q, want %q", string(got), string(next))
	}
}

func TestSaveRawAtomicValidatesBeforeWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.yaml")
	raw := []byte("domain: example.com\nport: 8080\nclients:\n  - clientId: a\n    clientSecret: b\n")
	if err := SaveRawAtomic(path, raw); err != nil {
		t.Fatalf("SaveRawAtomic valid config: %v", err)
	}
	if err := SaveRawAtomic(path, []byte("domain: example.com\n")); err == nil {
		t.Fatal("expected validation error for missing clients")
	}
}
