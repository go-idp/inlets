package main

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-zoox/inlets/internal/client"
)

const (
	ClientVersion = "2.0.0"
)

var portOnlyRegex = regexp.MustCompile(`^\d+$`)

func main() {
	// Parse flags first (they can appear anywhere)
	var port int
	var subDomain string
	var token string
	var credentials string
	var remote string
	var remoteTCPPort int = 8443
	var healthcheckInt int = 30000
	var reportURL string
	var legacy bool

	// Get defaults from environment
	if portStr := os.Getenv("TUNNEL_PORT"); portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil {
			port = p
		}
	}
	subDomain = os.Getenv("SUB_DOMAIN")
	token = os.Getenv("TOKEN")
	credentials = os.Getenv("CREDENTIALS")
	remote = os.Getenv("REMOTE")
	if remote == "" {
		remote = "inlets.zcorky.com:443"
	}
	if portStr := os.Getenv("REMOTE_TCP_PORT"); portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil {
			remoteTCPPort = p
		}
	}
	if intervalStr := os.Getenv("HEALTHCHECK_INTERVAL"); intervalStr != "" {
		if i, err := strconv.Atoi(intervalStr); err == nil {
			healthcheckInt = i
		}
	}
	reportURL = os.Getenv("REPORT_URL")
	if legacyStr := os.Getenv("LEGACY"); legacyStr != "" {
		legacy = legacyStr == "true" || legacyStr == "1" || legacyStr == "yes"
	}

	// Parse command line arguments to extract flags and positional args
	var tunnelType string
	var upstreamArg string
	var positionalArgs []string

	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		switch arg {
		case "-p", "--port":
			if i+1 < len(os.Args) {
				if p, err := strconv.Atoi(os.Args[i+1]); err == nil {
					port = p
				}
				i++
			}
		case "-s", "--sub-domain":
			if i+1 < len(os.Args) {
				subDomain = os.Args[i+1]
				i++
			}
		case "-t", "--token":
			if i+1 < len(os.Args) {
				token = os.Args[i+1]
				i++
			}
		case "--credentials":
			if i+1 < len(os.Args) {
				credentials = os.Args[i+1]
				i++
			}
		case "-r", "--remote":
			if i+1 < len(os.Args) {
				remote = os.Args[i+1]
				i++
			}
		case "--remote-tcp-port":
			if i+1 < len(os.Args) {
				if p, err := strconv.Atoi(os.Args[i+1]); err == nil {
					remoteTCPPort = p
				}
				i++
			}
		case "--healthcheck-interval":
			if i+1 < len(os.Args) {
				if interval, err := strconv.Atoi(os.Args[i+1]); err == nil {
					healthcheckInt = interval
				}
				i++
			}
		case "--report-url":
			if i+1 < len(os.Args) {
				reportURL = os.Args[i+1]
				i++
			}
		case "--legacy":
			legacy = true
		case "-v", "--version":
			fmt.Printf("%s\n", ClientVersion)
			os.Exit(0)
		case "-h", "--help":
			printHelp()
			os.Exit(0)
		default:
			// Not a flag, treat as positional argument
			if arg != "" && !strings.HasPrefix(arg, "-") {
				positionalArgs = append(positionalArgs, arg)
			}
		}
	}

	// Extract tunnel type and upstream from positional args
	if len(positionalArgs) < 2 {
		printHelp()
		os.Exit(1)
	}

	tunnelType = positionalArgs[0]
	upstreamArg = positionalArgs[1]

	// Validate type
	if tunnelType != "http" && tunnelType != "tcp" {
		fmt.Fprintf(os.Stderr, "Error: type must be 'http' or 'tcp'\n")
		os.Exit(1)
	}

	// Validate upstream
	upstreamRegex := regexp.MustCompile(`^(\d+|.+:\d+)$`)
	if !upstreamRegex.MatchString(upstreamArg) {
		fmt.Fprintf(os.Stderr, "Error: upstream must be port or hostname:port, such as 9000 or 127.0.0.1:9000\n")
		os.Exit(1)
	}

	// Parse upstream
	var upstreamHost string
	var upstreamPort int
	var err error

	if portOnlyRegex.MatchString(upstreamArg) {
		upstreamPort, err = strconv.Atoi(upstreamArg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: invalid port: %v\n", err)
			os.Exit(1)
		}
		upstreamHost = "127.0.0.1"
	} else {
		parts := strings.Split(upstreamArg, ":")
		if len(parts) != 2 {
			fmt.Fprintf(os.Stderr, "Error: invalid upstream format\n")
			os.Exit(1)
		}
		upstreamHost = parts[0]
		upstreamPort, err = strconv.Atoi(parts[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: invalid port: %v\n", err)
			os.Exit(1)
		}
	}

	// Determine auth type
	authType := "public"
	var clientId, clientSecret string

	if credentials != "" {
		parts := strings.Split(credentials, ":")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			fmt.Fprintf(os.Stderr, "Error: invalid credentials format, expected 'clientId:clientSecret'\n")
			os.Exit(1)
		}
		clientId = parts[0]
		clientSecret = parts[1]
		authType = "credentials"
	} else if token != "" {
		authType = "token"
	} else if tunnelType != "http" {
		fmt.Fprintf(os.Stderr, "Error: token or credentials is required for tcp tunnel\n")
		os.Exit(1)
	}

	if authType == "public" && tunnelType != "http" {
		fmt.Fprintf(os.Stderr, "Error: public auth only allowed for http\n")
		os.Exit(1)
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
	c := client.New(opts)
	if err := c.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Println(`inlets is a cloud native tunnel client that supports HTTP and TCP tunneling.

Usage:
  inlets-client [type] [upstream] [flags]

Examples:
  # HTTP tunnel
  inlets-client http 127.0.0.1:9000 -s myapp

  # TCP tunnel
  inlets-client tcp 127.0.0.1:22 -p 20100 -t your-token

  # TCP tunnel with credentials
  inlets-client tcp 127.0.0.1:22 --credentials clientId:clientSecret

Flags:
  -p, --port int                   Custom tunnel port for tcp (env: TUNNEL_PORT)
  -s, --sub-domain string          Custom tunnel sub domain for http (env: SUB_DOMAIN)
  -t, --token string               Authentication token (env: TOKEN)
      --credentials string         Authentication credentials (clientId:clientSecret) (env: CREDENTIALS)
  -r, --remote string              Server address (env: REMOTE) (default "inlets.zcorky.com:443")
      --remote-tcp-port int        Server tcp port (env: REMOTE_TCP_PORT) (default 8443)
      --healthcheck-interval int   Service health check interval (ms) (env: HEALTHCHECK_INTERVAL) (default 30000)
      --report-url string          Error report url (env: REPORT_URL)
      --legacy                     Use legacy protocol version (v1) (env: LEGACY)
  -v, --version                    Print version information and exit
  -h, --help                       help for inlets`)
}
