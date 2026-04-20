package main

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-idp/inlets/internal/client"
	"github.com/urfave/cli/v2"
)

const (
	ClientVersion = "2.0.0"
)

var portOnlyRegex = regexp.MustCompile(`^\d+$`)

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

Note: Flags must be placed before positional arguments.`,
		ArgsUsage: "[type] [upstream]",
		Flags: []cli.Flag{
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
			if c.NArg() < 2 {
				return cli.ShowAppHelp(c)
			}

			tunnelType := c.Args().Get(0)
			upstreamArg := c.Args().Get(1)

			// Get flags using urfave/cli/v2 standard way
			port := c.Int("port")
			subDomain := c.String("sub-domain")
			token := c.String("token")
			credentials := c.String("credentials")
			remote := c.String("remote")
			remoteTCPPort := c.Int("remote-tcp-port")
			healthcheckInt := c.Int("healthcheck-interval")
			reportURL := c.String("report-url")
			legacy := c.Bool("legacy")

			// Validate type
			if tunnelType != "http" && tunnelType != "tcp" {
				return fmt.Errorf("type must be 'http' or 'tcp'")
			}

			// Validate upstream
			upstreamRegex := regexp.MustCompile(`^(\d+|.+:\d+)$`)
			if !upstreamRegex.MatchString(upstreamArg) {
				return fmt.Errorf("upstream must be port or hostname:port, such as 9000 or 127.0.0.1:9000")
			}

			// Parse upstream
			var upstreamHost string
			var upstreamPort int
			var err error

			if portOnlyRegex.MatchString(upstreamArg) {
				upstreamPort, err = strconv.Atoi(upstreamArg)
				if err != nil {
					return fmt.Errorf("invalid port: %v", err)
				}
				upstreamHost = "127.0.0.1"
			} else {
				parts := strings.Split(upstreamArg, ":")
				if len(parts) != 2 {
					return fmt.Errorf("invalid upstream format")
				}
				upstreamHost = parts[0]
				upstreamPort, err = strconv.Atoi(parts[1])
				if err != nil {
					return fmt.Errorf("invalid port: %v", err)
				}
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
