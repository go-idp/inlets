package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadClientConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "client.yaml")
	err := os.WriteFile(cfgPath, []byte(`
type: http
upstream: 127.0.0.1:9000
credentials: client1:secret1
remote: 127.0.0.1:8080
`), 0o644)
	if err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := loadClientConfig(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Type != "http" || cfg.Upstream != "127.0.0.1:9000" || cfg.Credentials != "client1:secret1" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestLoadClientConfigHTTPAuthNested(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "client.yaml")
	err := os.WriteFile(cfgPath, []byte(`
type: http
upstream: 127.0.0.1:9000
credentials: client1:secret1
http:
  auth:
    username: u1
    password: p1
`), 0o644)
	if err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := loadClientConfig(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.HTTP == nil || cfg.HTTP.Auth == nil {
		t.Fatal("expected http.auth")
	}
	if cfg.HTTP.Auth.Username != "u1" || cfg.HTTP.Auth.Password != "p1" {
		t.Fatalf("unexpected http.auth: %+v", cfg.HTTP.Auth)
	}
}

func TestLoadClientConfigHTTPSubDomain(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "client.yaml")
	err := os.WriteFile(cfgPath, []byte(`
type: http
upstream: 127.0.0.1:9000
http:
  subDomain: app1
`), 0o644)
	if err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := loadClientConfig(cfgPath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.HTTP == nil || cfg.HTTP.SubDomain != "app1" {
		t.Fatalf("expected http.subDomain app1, got %+v", cfg.HTTP)
	}
}

func TestParseUpstreamArg(t *testing.T) {
	host, port, err := parseUpstreamArg("9000")
	if err != nil {
		t.Fatalf("parse port-only: %v", err)
	}
	if host != "127.0.0.1" || port != 9000 {
		t.Fatalf("unexpected parse result: %s:%d", host, port)
	}

	host, port, err = parseUpstreamArg("10.0.0.2:8081")
	if err != nil {
		t.Fatalf("parse host:port: %v", err)
	}
	if host != "10.0.0.2" || port != 8081 {
		t.Fatalf("unexpected parse result: %s:%d", host, port)
	}
}
