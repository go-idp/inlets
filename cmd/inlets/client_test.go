package main

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/urfave/cli/v2"
)

func intFlagNames(flags []cli.Flag) []string {
	var out []string
	for _, f := range flags {
		if fl, ok := f.(*cli.IntFlag); ok {
			out = append(out, fl.Name)
		}
	}
	return out
}

func hasIntFlagName(flags []cli.Flag, name string) bool {
	for _, n := range intFlagNames(flags) {
		if n == name {
			return true
		}
	}
	return false
}

func testClientRootContext(t *testing.T, argv []string) *cli.Context {
	t.Helper()
	cmd := Client()
	set := flag.NewFlagSet("test", flag.ContinueOnError)
	for _, f := range cmd.Flags {
		if err := f.Apply(set); err != nil {
			t.Fatal(err)
		}
	}
	if err := set.Parse(argv); err != nil {
		t.Fatal(err)
	}
	ctx := cli.NewContext(nil, set, nil)
	ctx.Command = cmd
	return ctx
}

func testClientHTTPContext(t *testing.T, argv []string) *cli.Context {
	t.Helper()
	cmd := Client()
	var httpCmd *cli.Command
	for _, c := range cmd.Subcommands {
		if c.Name == "http" {
			httpCmd = c
			break
		}
	}
	if httpCmd == nil {
		t.Fatal("missing http subcommand")
	}

	parentSet := flag.NewFlagSet("client", flag.ContinueOnError)
	for _, f := range cmd.Flags {
		if err := f.Apply(parentSet); err != nil {
			t.Fatal(err)
		}
	}
	parentCtx := cli.NewContext(nil, parentSet, nil)
	parentCtx.Command = cmd

	leafSet := flag.NewFlagSet("http", flag.ContinueOnError)
	for _, f := range httpCmd.Flags {
		if err := f.Apply(leafSet); err != nil {
			t.Fatal(err)
		}
	}
	if err := leafSet.Parse(argv); err != nil {
		t.Fatal(err)
	}
	leafCtx := cli.NewContext(nil, leafSet, parentCtx)
	leafCtx.Command = httpCmd
	return leafCtx
}

func TestBuildClientOptions_CoordinatorMode(t *testing.T) {
	t.Parallel()

	ctx := testClientRootContext(t, []string{
		"--client-id", "c1",
		"--client-secret", "s1",
		"--server", "https://example.com",
	})
	built, err := buildClientOptions(ctx, "")
	if err != nil {
		t.Fatalf("buildClientOptions: %v", err)
	}
	if built.Type != "" {
		t.Fatalf("coordinator mode should not set tunnel type, got %q", built.Type)
	}
	if built.UpstreamPort != 0 {
		t.Fatalf("coordinator mode should not set upstream port, got %d", built.UpstreamPort)
	}
}

func TestBuildClientOptions_HTTPSubcommand(t *testing.T) {
	t.Parallel()

	ctx := testClientHTTPContext(t, []string{"127.0.0.1:9000"})
	built, err := buildClientOptions(ctx, "http")
	if err != nil {
		t.Fatalf("buildClientOptions: %v", err)
	}
	if built.Type != "http" || built.UpstreamPort != 9000 {
		t.Fatalf("unexpected http options: type=%q port=%d", built.Type, built.UpstreamPort)
	}
}

func TestResolveClientAuth(t *testing.T) {
	t.Parallel()

	t.Run("id_secret_pair_takes_precedence", func(t *testing.T) {
		t.Parallel()
		at, id, sec, err := resolveClientAuth("c1", "s1", "other:other", "tk")
		if err != nil {
			t.Fatal(err)
		}
		if at != "credentials" || id != "c1" || sec != "s1" {
			t.Fatalf("got %s %q %q", at, id, sec)
		}
	})
	t.Run("credentials_when_no_pair", func(t *testing.T) {
		t.Parallel()
		at, id, sec, err := resolveClientAuth("", "", "a:b", "")
		if err != nil {
			t.Fatal(err)
		}
		if at != "credentials" || id != "a" || sec != "b" {
			t.Fatalf("got %s %q %q", at, id, sec)
		}
	})
	t.Run("credentials_secret_may_contain_colons", func(t *testing.T) {
		t.Parallel()
		at, id, sec, err := resolveClientAuth("", "", "myid:sec:with:colons", "")
		if err != nil {
			t.Fatal(err)
		}
		if at != "credentials" || id != "myid" || sec != "sec:with:colons" {
			t.Fatalf("got %s %q %q", at, id, sec)
		}
	})
	t.Run("token_when_no_creds", func(t *testing.T) {
		t.Parallel()
		at, id, sec, err := resolveClientAuth("", "", "", "t1")
		if err != nil {
			t.Fatal(err)
		}
		if at != "token" || id != "" || sec != "" {
			t.Fatalf("got %s %q %q", at, id, sec)
		}
	})
	t.Run("incomplete_id_secret", func(t *testing.T) {
		t.Parallel()
		_, _, _, err := resolveClientAuth("c1", "", "a:b", "")
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("public", func(t *testing.T) {
		t.Parallel()
		at, _, _, err := resolveClientAuth("", "", "", "")
		if err != nil {
			t.Fatal(err)
		}
		if at != "public" {
			t.Fatalf("got %q", at)
		}
	})
}

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
		name                string
		legacy              bool
		serverConfigured    bool
		remoteConfigured    bool
		remoteTCPConfigured bool
		wantErr             bool
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

func TestClientRootHasNoTunnelPortFlag(t *testing.T) {
	t.Parallel()

	root := Client()
	if hasIntFlagName(root.Flags, "port") {
		t.Fatal("client root must not register -p/--port (use tcp subcommand)")
	}

	var tcpCmd *cli.Command
	for _, c := range root.Subcommands {
		if c.Name == "tcp" {
			tcpCmd = c
			break
		}
	}
	if tcpCmd == nil {
		t.Fatal("expected tcp subcommand")
	}
	if !hasIntFlagName(tcpCmd.Flags, "port") {
		t.Fatal("tcp subcommand must register --port for public listen port on server")
	}
	var portFL *cli.IntFlag
	for _, f := range tcpCmd.Flags {
		if fl, ok := f.(*cli.IntFlag); ok && fl.Name == "port" {
			portFL = fl
			break
		}
	}
	if portFL == nil {
		t.Fatal("expected *cli.IntFlag port on tcp")
	}
	if len(portFL.Aliases) != 1 || portFL.Aliases[0] != "p" {
		t.Fatalf("tcp --port aliases: got %#v, want [p]", portFL.Aliases)
	}
}

func TestClientPortFlagMustFollowTCPSubcommand(t *testing.T) {
	t.Parallel()

	app := &cli.App{
		Name:     "inlets",
		Commands: []*cli.Command{Client()},
	}

	err := app.Run([]string{"inlets", "client", "-p", "20100", "http", "127.0.0.1:9000"})
	if err == nil {
		t.Fatalf("expected error for client-level -p before http, got nil")
	}
	if !strings.Contains(err.Error(), "p") {
		t.Fatalf("expected flag error mentioning -p, got: %v", err)
	}
}

func TestLoadClientConfigTCPNestedPort(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "client.yaml")
	err := os.WriteFile(cfgPath, []byte(`
type: tcp
upstream: 127.0.0.1:22
credentials: a:b
tcp:
  port: 20100
`), 0o644)
	if err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := loadClientConfig(cfgPath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.TCP == nil || cfg.TCP.Port != 20100 {
		t.Fatalf("expected tcp.port 20100, got %+v", cfg.TCP)
	}
}

func TestLoadClientConfigTCPLegacyTopLevelPort(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "client.yaml")
	err := os.WriteFile(cfgPath, []byte(`
type: tcp
upstream: 127.0.0.1:22
credentials: a:b
port: 20202
`), 0o644)
	if err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := loadClientConfig(cfgPath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Port != 20202 {
		t.Fatalf("expected legacy top-level port 20202, got %d", cfg.Port)
	}
}
