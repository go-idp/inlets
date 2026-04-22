package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/urfave/cli/v2"
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

func TestParseServerArg(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "http host and port", input: "http://example.com:8080", want: "http://example.com:8080"},
		{name: "http host and port with path", input: "http://example.com:8080/base/path", want: "http://example.com:8080/base/path"},
		{name: "http host default port", input: "http://example.com", want: "http://example.com:80"},
		{name: "http host default port with path", input: "http://example.com/base/path", want: "http://example.com:80/base/path"},
		{name: "https host default port", input: "https://example.com", want: "https://example.com:443"},
		{name: "https host and path", input: "https://example.com/base/path", want: "https://example.com:443/base/path"},
		{name: "missing scheme", input: "example.com", wantErr: true},
		{name: "unsupported scheme", input: "ws://example.com", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseServerArg(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseServerArg(%q) returned error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("parseServerArg(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidateTransportMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		legacy            bool
		serverConfigured  bool
		remoteConfigured  bool
		remoteTCPConfigured bool
		wantErr           bool
	}{
		{name: "v2 with server only", legacy: false, serverConfigured: true, wantErr: false},
		{name: "legacy with remote", legacy: true, remoteConfigured: true, wantErr: false},
		{name: "legacy with remote tcp port", legacy: true, remoteTCPConfigured: true, wantErr: false},
		{name: "legacy with server should fail", legacy: true, serverConfigured: true, wantErr: true},
		{name: "v2 with remote should fail", legacy: false, remoteConfigured: true, wantErr: true},
		{name: "v2 with remote tcp port should fail", legacy: false, remoteTCPConfigured: true, wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateTransportMode(tt.legacy, tt.serverConfigured, tt.remoteConfigured, tt.remoteTCPConfigured)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
		})
	}
}

func TestSubDomainFlagMustFollowHTTPSubcommand(t *testing.T) {
	t.Parallel()

	app := &cli.App{
		Name:     "inlets",
		Commands: []*cli.Command{Client()},
	}

	err := app.Run([]string{"inlets", "client", "--sub-domain", "myapp", "http", "127.0.0.1:9000"})
	if err == nil {
		t.Fatalf("expected parse error for client-level --sub-domain, got nil")
	}
	if !strings.Contains(err.Error(), "sub-domain") {
		t.Fatalf("expected sub-domain parse error, got: %v", err)
	}
}
