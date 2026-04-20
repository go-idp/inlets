package main

import (
	"fmt"
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

type ClientFileConfig struct {
	Type           string `yaml:"type"`
	Upstream       string `yaml:"upstream"`
	Port           int    `yaml:"port"`
	SubDomain      string `yaml:"subDomain"`
	Token          string `yaml:"token"`
	Credentials    string `yaml:"credentials"`
	Remote         string `yaml:"remote"`
	RemoteTCPPort  int    `yaml:"remoteTCPPort"`
	HealthcheckInt int    `yaml:"healthcheckInterval"`
	ReportURL      string `yaml:"reportURL"`
	Legacy         bool   `yaml:"legacy"`
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

func Client() *cli.Command {
	return &cli.Command{
		Name:  "client",
		Usage: "inlets tunnel client",
		Description: `inlets is a cloud native tunnel client that supports HTTP and TCP tunneling.

Examples:
  # HTTP tunnel (flags before positional args)
  inlets client -s myapp http 127.0.0.1:9000

  # TCP tunnel (flags before positional args)
  inlets client -p 20100 -t your-token tcp 127.0.0.1:22

  # TCP tunnel with credentials (flags before positional args)
  inlets client --credentials clientId:clientSecret -p 20100 tcp 127.0.0.1:22

  # Server-managed tunnels only (no positional args)
  inlets client --credentials clientId:clientSecret

  # From config file
  inlets client -c ./conf/example/client.yaml

Note: Flags must be placed before positional arguments.`,
		ArgsUsage: "[type] [upstream]",
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
				Name:    "sub-domain",
				Aliases: []string{"s"},
				Usage:   "Custom tunnel sub domain for http (env: SUB_DOMAIN)",
				EnvVars: []string{"SUB_DOMAIN"},
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
				Name:    "remote",
				Aliases: []string{"r"},
				Usage:   "Server address (env: REMOTE)",
				Value:   "inlets.zcorky.com:443",
				EnvVars: []string{"REMOTE"},
			},
			&cli.IntFlag{
				Name:    "remote-tcp-port",
				Usage:   "Server tcp port (env: REMOTE_TCP_PORT)",
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
		Action: func(c *cli.Context) error {
			if c.NArg() != 0 && c.NArg() != 2 {
				return cli.ShowAppHelp(c)
			}

			tunnelType := "http"
			upstreamHost := "127.0.0.1"
			upstreamPort := 80
			port := 0
			subDomain := ""
			token := ""
			credentials := ""
			remote := "inlets.zcorky.com:443"
			remoteTCPPort := 8443
			healthcheckInt := 30000
			reportURL := ""
			legacy := false

			configPath := c.String("config")
			if strings.TrimSpace(configPath) != "" {
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
				subDomain = strings.TrimSpace(cfg.SubDomain)
				token = strings.TrimSpace(cfg.Token)
				credentials = strings.TrimSpace(cfg.Credentials)
				if strings.TrimSpace(cfg.Remote) != "" {
					remote = strings.TrimSpace(cfg.Remote)
				}
				if cfg.RemoteTCPPort > 0 {
					remoteTCPPort = cfg.RemoteTCPPort
				}
				if cfg.HealthcheckInt > 0 {
					healthcheckInt = cfg.HealthcheckInt
				}
				reportURL = strings.TrimSpace(cfg.ReportURL)
				legacy = cfg.Legacy
			}

			// Merge CLI flags (CLI has highest priority).
			if c.IsSet("port") {
				port = c.Int("port")
			}
			if c.IsSet("sub-domain") {
				subDomain = c.String("sub-domain")
			}
			if c.IsSet("token") {
				token = c.String("token")
			}
			if c.IsSet("credentials") {
				credentials = c.String("credentials")
			}
			if c.IsSet("remote") {
				remote = c.String("remote")
			}
			if c.IsSet("remote-tcp-port") {
				remoteTCPPort = c.Int("remote-tcp-port")
			}
			if c.IsSet("healthcheck-interval") {
				healthcheckInt = c.Int("healthcheck-interval")
			}
			if c.IsSet("report-url") {
				reportURL = c.String("report-url")
			}
			if c.IsSet("legacy") {
				legacy = c.Bool("legacy")
			}

			if c.NArg() == 2 {
				tunnelType = c.Args().Get(0)
				upstreamArg := c.Args().Get(1)

				// Validate type
				if tunnelType != "http" && tunnelType != "tcp" {
					return fmt.Errorf("type must be 'http' or 'tcp'")
				}

				var err error
				upstreamHost, upstreamPort, err = parseUpstreamArg(upstreamArg)
				if err != nil {
					return err
				}
			} else if strings.TrimSpace(credentials) == "" {
				return fmt.Errorf("when type/upstream is omitted, --credentials is required to use server-managed tunnels")
			}

			if tunnelType != "http" && tunnelType != "tcp" {
				return fmt.Errorf("type must be 'http' or 'tcp'")
			}

			// Determine auth type
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

			// Determine protocol version: v2 (2.0.0) by default, v1 (1.2.0) if legacy
			protocolVersion := "2.0.0"
			if legacy {
				protocolVersion = "1.2.0"
			}

			// Create client options
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
				Remote:         remote,
				RemoteTCPPort:  remoteTCPPort,
				HealthcheckInt: healthcheckInt,
				ReportURL:      reportURL,
				Version:        protocolVersion,
			}

			// Create and run client
			cl := client.New(opts)
			if err := cl.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				return err
			}

			return nil
		},
	}
}
