package main

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-idp/inlets/internal/client"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
)

const (
	ClientVersion = "2.0.0"
)

var portOnlyRegex = regexp.MustCompile(`^\d+$`)

// ClientHTTPAuthConfig holds optional credentials for dialing the local HTTP upstream (Basic auth).
type ClientHTTPAuthConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// ClientHTTPConfig holds HTTP tunnel–specific client options.
type ClientHTTPConfig struct {
	SubDomain string                `yaml:"subDomain,omitempty"`
	Auth      *ClientHTTPAuthConfig `yaml:"auth,omitempty"`
}

type ClientFileConfig struct {
	Type           string              `yaml:"type"`
	Upstream       string              `yaml:"upstream"`
	Port           int                 `yaml:"port"`
	Token          string              `yaml:"token"`
	Credentials    string              `yaml:"credentials"`
	HTTP           *ClientHTTPConfig   `yaml:"http,omitempty"`
	Server         string              `yaml:"server"`
	Remote         string              `yaml:"remote"`
	RemoteTCPPort  int                 `yaml:"remoteTCPPort"`
	HealthcheckInt int                 `yaml:"healthcheckInterval"`
	ReportURL      string              `yaml:"reportURL"`
	Legacy         bool                `yaml:"legacy"`
}

func loadClientConfig(path string) (*ClientFileConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read client config file: %w", err)
	}
	var cfg ClientFileConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse client config file: %w", err)
	}
	return &cfg, nil
}

func parseUpstreamArg(upstreamArg string) (string, int, error) {
	upstreamRegex := regexp.MustCompile(`^(\d+|.+:\d+)$`)
	if !upstreamRegex.MatchString(upstreamArg) {
		return "", 0, fmt.Errorf("upstream must be port or hostname:port, such as 9000 or 127.0.0.1:9000")
	}

	if portOnlyRegex.MatchString(upstreamArg) {
		upstreamPort, err := strconv.Atoi(upstreamArg)
		if err != nil {
			return "", 0, fmt.Errorf("invalid port: %v", err)
		}
		return "127.0.0.1", upstreamPort, nil
	}

	parts := strings.Split(upstreamArg, ":")
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("invalid upstream format")
	}
	upstreamPort, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, fmt.Errorf("invalid port: %v", err)
	}
	return parts[0], upstreamPort, nil
}

func parseServerArg(serverArg string) (string, error) {
	raw := strings.TrimSpace(serverArg)
	if raw == "" {
		return "", fmt.Errorf("--server cannot be empty")
	}

	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid --server value: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("--server must be a full URL (for example: https://example.com or https://example.com/base)")
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("--server only supports http:// or https:// URLs")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("--server must not include query string or fragment")
	}

	host := u.Hostname()
	if host == "" {
		return "", fmt.Errorf("--server host is required")
	}

	port := u.Port()
	if port == "" {
		if scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}

	path := strings.TrimSuffix(u.EscapedPath(), "/")
	if path == "/" {
		path = ""
	}

	normalized := &url.URL{
		Scheme: scheme,
		Host:   net.JoinHostPort(host, port),
		Path:   path,
	}

	return normalized.String(), nil
}

func validateTransportMode(legacy, serverConfigured, remoteConfigured, remoteTCPConfigured bool) error {
	if serverConfigured && legacy {
		return fmt.Errorf("--server only supports v2 protocol; remove --legacy")
	}
	if !legacy && (remoteConfigured || remoteTCPConfigured) {
		return fmt.Errorf("--remote and --remote-tcp-port only support legacy mode; for v2 use --server")
	}
	return nil
}

// resolveClientCtx returns the parent "client" command context (flags: token, credentials, etc.).
func resolveClientCtx(leaf *cli.Context) *cli.Context {
	for _, x := range leaf.Lineage() {
		if x.Command != nil && x.Command.Name == "client" {
			return x
		}
	}
	return leaf
}

func runTunnelClient(leaf *cli.Context, subcommandType string) error {
	cc := resolveClientCtx(leaf)

	tunnelType := "http"
	upstreamHost := "127.0.0.1"
	upstreamPort := 80
	port := 0
	subDomain := ""
	token := ""
	credentials := ""
	server := ""
	remote := "inlets.zcorky.com:443"
	remoteTCPPort := 8443
	healthcheckInt := 30000
	reportURL := ""
	legacy := false
	upstreamUser := ""
	upstreamPass := ""
	serverConfigured := false
	remoteConfigured := false
	remoteTCPConfigured := false

	configPath := strings.TrimSpace(cc.String("config"))
	if configPath != "" {
		cfg, err := loadClientConfig(configPath)
		if err != nil {
			return err
		}
		if strings.TrimSpace(cfg.Type) != "" {
			tunnelType = strings.TrimSpace(cfg.Type)
		}
		if strings.TrimSpace(cfg.Upstream) != "" {
			h, p, err := parseUpstreamArg(strings.TrimSpace(cfg.Upstream))
			if err != nil {
				return err
			}
			upstreamHost = h
			upstreamPort = p
		}
		if cfg.Port > 0 {
			port = cfg.Port
		}
		token = strings.TrimSpace(cfg.Token)
		credentials = strings.TrimSpace(cfg.Credentials)
		if strings.TrimSpace(cfg.Server) != "" {
			server = strings.TrimSpace(cfg.Server)
			serverConfigured = true
		}
		if strings.TrimSpace(cfg.Remote) != "" {
			remote = strings.TrimSpace(cfg.Remote)
			remoteConfigured = true
		}
		if cfg.RemoteTCPPort > 0 {
			remoteTCPPort = cfg.RemoteTCPPort
			remoteTCPConfigured = true
		}
		if cfg.HealthcheckInt > 0 {
			healthcheckInt = cfg.HealthcheckInt
		}
		reportURL = strings.TrimSpace(cfg.ReportURL)
		legacy = cfg.Legacy
		cfgType := strings.TrimSpace(cfg.Type)
		if cfg.HTTP != nil && cfg.HTTP.Auth != nil {
			upstreamUser = strings.TrimSpace(cfg.HTTP.Auth.Username)
			upstreamPass = strings.TrimSpace(cfg.HTTP.Auth.Password)
		}
		if cfg.HTTP != nil && (cfgType == "" || strings.EqualFold(cfgType, "http")) {
			subDomain = strings.TrimSpace(cfg.HTTP.SubDomain)
		}
	}

	if cc.IsSet("port") {
		port = cc.Int("port")
	}
	if cc.IsSet("token") {
		token = cc.String("token")
	}
	if cc.IsSet("credentials") {
		credentials = cc.String("credentials")
	}
	if cc.IsSet("server") {
		server = cc.String("server")
		serverConfigured = true
	}
	if cc.IsSet("remote") {
		remote = cc.String("remote")
		remoteConfigured = true
	}
	if cc.IsSet("remote-tcp-port") {
		remoteTCPPort = cc.Int("remote-tcp-port")
		remoteTCPConfigured = true
	}
	if cc.IsSet("healthcheck-interval") {
		healthcheckInt = cc.Int("healthcheck-interval")
	}
	if cc.IsSet("report-url") {
		reportURL = cc.String("report-url")
	}
	if cc.IsSet("legacy") {
		legacy = cc.Bool("legacy")
	}

	if serverConfigured {
		normalizedServer, err := parseServerArg(server)
		if err != nil {
			return err
		}
		server = normalizedServer
	}
	if err := validateTransportMode(legacy, serverConfigured, remoteConfigured, remoteTCPConfigured); err != nil {
		return err
	}
	if !legacy {
		if !serverConfigured {
			server = "https://inlets.zcorky.com"
		}
	}

	switch subcommandType {
	case "http", "tcp":
		if leaf.NArg() != 1 {
			return fmt.Errorf("%s requires exactly one upstream argument (port or host:port)", subcommandType)
		}
		h, p, err := parseUpstreamArg(leaf.Args().First())
		if err != nil {
			return err
		}
		upstreamHost, upstreamPort = h, p
		tunnelType = subcommandType
		if subcommandType == "http" {
			if leaf.IsSet("sub-domain") {
				subDomain = leaf.String("sub-domain")
			}
			if leaf.IsSet("username") {
				upstreamUser = leaf.String("username")
			}
			if leaf.IsSet("password") {
				upstreamPass = leaf.String("password")
			}
		}
	default:
		if strings.TrimSpace(credentials) == "" {
			return fmt.Errorf("when http/tcp subcommand is omitted, --credentials is required to use server-managed tunnels")
		}
	}

	if tunnelType != "http" && tunnelType != "tcp" {
		return fmt.Errorf("type must be 'http' or 'tcp'")
	}

	// Parent-only + YAML http: SUB_DOMAIN is not parsed as an http subcommand flag; honor env here.
	if tunnelType == "http" && strings.TrimSpace(subDomain) == "" && subcommandType == "" {
		subDomain = strings.TrimSpace(os.Getenv("SUB_DOMAIN"))
	}

	if tunnelType != "http" {
		subDomain = ""
		upstreamUser = ""
		upstreamPass = ""
	}

	authType := "public"
	var clientId, clientSecret string

	if credentials != "" {
		parts := strings.Split(credentials, ":")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return fmt.Errorf("invalid credentials format, expected 'clientId:clientSecret'")
		}
		clientId = parts[0]
		clientSecret = parts[1]
		authType = "credentials"
	} else if token != "" {
		authType = "token"
	} else if tunnelType != "http" {
		return fmt.Errorf("token or credentials is required for tcp tunnel")
	}

	if authType == "public" && tunnelType != "http" {
		return fmt.Errorf("public auth only allowed for http")
	}

	protocolVersion := "2.0.0"
	if legacy {
		protocolVersion = "1.2.0"
	}

	opts := &client.Options{
		Type:           tunnelType,
		UpstreamHost:   upstreamHost,
		UpstreamPort:   upstreamPort,
		AuthType:       authType,
		Token:          token,
		ClientId:       clientId,
		ClientSecret:   clientSecret,
		SubDomain:      subDomain,
		Port:           port,
		Server:         server,
		Remote:         remote,
		RemoteTCPPort:  remoteTCPPort,
		HealthcheckInt: healthcheckInt,
		ReportURL:      reportURL,
		Version:        protocolVersion,
	}
	if tunnelType == "http" {
		opts.UpstreamUsername = upstreamUser
		opts.UpstreamPassword = upstreamPass
	}

	cl := client.New(opts)
	if err := cl.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return err
	}

	return nil
}

func Client() *cli.Command {
	httpCmd := &cli.Command{
		Name:      "http",
		Usage:     "Expose an HTTP upstream through the tunnel",
		ArgsUsage: "[upstream]",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "sub-domain",
				Usage:   "Public HTTP tunnel sub domain (env: SUB_DOMAIN)",
				EnvVars: []string{"SUB_DOMAIN"},
			},
			&cli.StringFlag{
				Name:    "username",
				Usage:   "HTTP Basic username for the local upstream (env: UPSTREAM_HTTP_USERNAME)",
				EnvVars: []string{"UPSTREAM_HTTP_USERNAME"},
			},
			&cli.StringFlag{
				Name:    "password",
				Usage:   "HTTP Basic password for the local upstream (env: UPSTREAM_HTTP_PASSWORD)",
				EnvVars: []string{"UPSTREAM_HTTP_PASSWORD"},
			},
		},
		Action: func(c *cli.Context) error {
			return runTunnelClient(c, "http")
		},
	}

	tcpCmd := &cli.Command{
		Name:      "tcp",
		Usage:     "Expose a TCP upstream through the tunnel",
		ArgsUsage: "[upstream]",
		Action: func(c *cli.Context) error {
			return runTunnelClient(c, "tcp")
		},
	}

	return &cli.Command{
		Name:  "client",
		Usage: "inlets tunnel client",
		Description: `inlets is a cloud native tunnel client that supports HTTP and TCP tunneling.

Examples:
  # v2 client via URL-style server endpoint
  inlets client -s https://tunnel.example.com http --sub-domain myapp 127.0.0.1:9000

  # HTTP tunnel (--sub-domain belongs to the http subcommand)
  inlets client http --sub-domain myapp 127.0.0.1:9000

  # HTTP upstream Basic auth
  inlets client http 127.0.0.1:9000 --username admin --password secret

  # TCP tunnel
  inlets client -p 20100 -t your-token tcp 127.0.0.1:22

  # TCP tunnel with credentials
  inlets client --credentials clientId:clientSecret -p 20100 tcp 127.0.0.1:22

  # Server-managed tunnels only (no http/tcp subcommand)
  inlets client --credentials clientId:clientSecret

  # From config file
  inlets client -c ./conf/example/client.yaml

Note: Global client flags (--token, --credentials, -s/--server, etc.) belong before the subcommand; HTTP-only flags (--sub-domain, upstream Basic auth) after "http", e.g. "inlets client -t TOKEN -s https://tunnel.example.com http --sub-domain myapp 9000".

Transport mode note:
  - v2: use --server (supports http://host:port, http://host, https://host, https://host/path)
  - legacy: use --remote / --remote-tcp-port with --legacy`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "Path to client config YAML file",
			},
			&cli.IntFlag{
				Name:    "port",
				Aliases: []string{"p"},
				Usage:   "Custom tunnel port for tcp (env: TUNNEL_PORT)",
				EnvVars: []string{"TUNNEL_PORT"},
			},
			&cli.StringFlag{
				Name:    "token",
				Aliases: []string{"t"},
				Usage:   "Authentication token (env: TOKEN)",
				EnvVars: []string{"TOKEN"},
			},
			&cli.StringFlag{
				Name:    "credentials",
				Usage:   "Authentication credentials (clientId:clientSecret) (env: CREDENTIALS)",
				EnvVars: []string{"CREDENTIALS"},
			},
			&cli.StringFlag{
				Name:    "server",
				Aliases: []string{"s"},
				Usage:   "v2 server URL (http/https, optional path) (env: SERVER)",
				Value:   "https://inlets.zcorky.com",
				EnvVars: []string{"SERVER"},
			},
			&cli.StringFlag{
				Name:    "remote",
				Aliases: []string{"r"},
				Usage:   "Legacy mode server address host:port (env: REMOTE)",
				Value:   "inlets.zcorky.com:443",
				EnvVars: []string{"REMOTE"},
			},
			&cli.IntFlag{
				Name:    "remote-tcp-port",
				Usage:   "Legacy mode server tcp port (env: REMOTE_TCP_PORT)",
				Value:   8443,
				EnvVars: []string{"REMOTE_TCP_PORT"},
			},
			&cli.IntFlag{
				Name:    "healthcheck-interval",
				Usage:   "Service health check interval (ms) (env: HEALTHCHECK_INTERVAL)",
				Value:   30000,
				EnvVars: []string{"HEALTHCHECK_INTERVAL"},
			},
			&cli.StringFlag{
				Name:    "report-url",
				Usage:   "Error report url (env: REPORT_URL)",
				EnvVars: []string{"REPORT_URL"},
			},
			&cli.BoolFlag{
				Name:    "legacy",
				Usage:   "Use legacy protocol version (v1) (env: LEGACY)",
				EnvVars: []string{"LEGACY"},
			},
		},
		Subcommands: []*cli.Command{httpCmd, tcpCmd},
		Action: func(c *cli.Context) error {
			return runTunnelClient(c, "")
		},
	}
}
