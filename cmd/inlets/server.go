package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"

	"github.com/go-idp/inlets/internal/client"
	"github.com/go-idp/inlets/internal/server"
	"github.com/go-zoox/logger"
	"github.com/go-idp/inlets/internal/server/limiter"
	"github.com/go-idp/inlets/internal/server/types"
	"github.com/urfave/cli/v2"
)

const (
	ServerVersion = "2.0.0"
)

// ConfigFilePaths defines the priority order of configuration file paths
// Paths with placeholders will be resolved at runtime:
// - {CWD} will be replaced with current working directory
// - {HOME} will be replaced with user home directory
var ConfigFilePaths = []string{
	"{CWD}/.go-idp/inlets.yaml",
	"{CWD}/.inlets.yaml",
	"{HOME}/.go-idp/inlets/config.yaml",
	"{HOME}/.config/inlets.yaml",
	"{HOME}/.config/inlets.yml",
	"/etc/go-idp/inlets/config.yaml",
	"/etc/inlets/config.yaml",
}

// ServerConfig represents the server configuration from YAML file
type ServerConfig struct {
	Domain          string                     `yaml:"domain"`
	Port            int                        `yaml:"port"`
	TCPPort         int                        `yaml:"tcpPort"`
	Secure          *bool                      `yaml:"secure"`
	Token           string                     `yaml:"token"`
	Clients         []ClientConfig             `yaml:"clients"`
	Notification    *client.NotificationConfig `yaml:"notification"`
	BandwidthLimits *BandwidthLimitsConfig     `yaml:"bandwidthLimits"`
}

// ClientConfig represents a client configuration
type ClientConfig struct {
	ClientID       string                  `yaml:"clientId"`
	ClientSecret   string                  `yaml:"clientSecret"`
	Config         *client.Config          `yaml:"config"`
	BandwidthLimit *limiter.BandwidthLimit `yaml:"bandwidthLimit"`
}

// BandwidthLimitsConfig represents bandwidth limits configuration
type BandwidthLimitsConfig struct {
	Global  *limiter.BandwidthLimit            `yaml:"global"`
	Clients map[string]*limiter.BandwidthLimit `yaml:"clients"`
}

func Server() *cli.Command {
	return &cli.Command{
		Name:  "server",
		Usage: "inlets tunnel server",
		Description: `Cloud Native Tunnel Server

  USAGE — server
  
    ▸ inlets server  <OPTIONS...>`,
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:    "port",
				Aliases: []string{"p"},
				Usage:   "Port for server (default 8080)",
				Value:   8080,
				EnvVars: []string{"SERVER_PORT"},
			},
			&cli.BoolFlag{
				Name:    "secure",
				Aliases: []string{"s"},
				Usage:   "Server with https, only for url (default: true)",
				Value:   true,
				EnvVars: []string{"SECURE"},
			},
			&cli.IntFlag{
				Name:    "tcp-port",
				Usage:   "TCP Port for server (default 8443)",
				Value:   8443,
				EnvVars: []string{"SERVER_TCP_PORT"},
			},
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "Config file (default: $HOME/.config/inlets.yml)",
			},
			&cli.StringFlag{
				Name:    "notification-provider",
				Usage:   "Notification provider (dingtalk, feishu, wecom, slack) (env: NOTIFICATION_PROVIDER)",
				EnvVars: []string{"NOTIFICATION_PROVIDER"},
			},
			&cli.StringFlag{
				Name:    "notification-url",
				Usage:   "Notification webhook URL (env: NOTIFICATION_URL)",
				EnvVars: []string{"NOTIFICATION_URL"},
			},
		},
		Action: func(c *cli.Context) error {
			// Get config path
			configPath := c.String("config")

			// If config path is not specified, find it by priority
			if configPath == "" {
				configPath = findConfigFile()
				if configPath == "" {
					// No config file found, prompt user
					fmt.Println("No configuration file found. Please configure /etc/go-idp/inlets/config.yaml")
				}
			}

			// Load config file if specified or found
			var configFile *ServerConfig
			if configPath != "" {
				loadedConfig, err := loadConfigFile(configPath)
				if err != nil {
					logger.Infof("[server] Warning: Failed to load config file %s: %v", configPath, err)
					return fmt.Errorf("failed to load config file: %v", err)
				} else if loadedConfig != nil {
					// Validate that Clients is not empty
					if len(loadedConfig.Clients) == 0 {
						return fmt.Errorf("config file %s: clients configuration is required and cannot be empty", configPath)
					}
					configFile = loadedConfig
					logger.Infof("[server] Config file loaded: %s", configPath)
				}
			}

			// Get values with priority: command line > environment variables > config file > defaults
			serverPort := c.Int("port")
			if serverPort == 0 {
				if configFile != nil && configFile.Port > 0 {
					serverPort = configFile.Port
				} else {
					serverPort = 8080
				}
			}

			serverTCPPort := c.Int("tcp-port")
			if serverTCPPort == 0 {
				if configFile != nil && configFile.TCPPort > 0 {
					serverTCPPort = configFile.TCPPort
				} else {
					serverTCPPort = 8443
				}
			}

			serverSecure := c.Bool("secure")
			if !serverSecure {
				if configFile != nil && configFile.Secure != nil {
					serverSecure = *configFile.Secure
				} else {
					serverSecure = true
				}
			}

			notificationProvider := c.String("notification-provider")
			if notificationProvider == "" {
				if configFile != nil && configFile.Notification != nil {
					notificationProvider = configFile.Notification.Provider
				}
			}

			notificationURL := c.String("notification-url")
			if notificationURL == "" {
				if configFile != nil && configFile.Notification != nil {
					notificationURL = configFile.Notification.URL
				}
			}

			// Setup notification config
			var notificationConfig *client.NotificationConfig
			if notificationProvider != "" && notificationURL != "" {
				notificationConfig = &client.NotificationConfig{
					Provider: notificationProvider,
					URL:      notificationURL,
				}
			} else if configFile != nil && configFile.Notification != nil {
				notificationConfig = configFile.Notification
			}

			// Setup bandwidth limits from config file
			var bandwidthLimits *limiter.ClientBandwidthLimits
			if configFile != nil && configFile.BandwidthLimits != nil {
				bandwidthLimits = &limiter.ClientBandwidthLimits{
					ByClientId: make(map[string]*limiter.BandwidthLimit),
				}
				if configFile.BandwidthLimits.Global != nil {
					bandwidthLimits.Global = configFile.BandwidthLimits.Global
				}
				if configFile.BandwidthLimits.Clients != nil {
					bandwidthLimits.ByClientId = configFile.BandwidthLimits.Clients
				}
				// Also add bandwidth limits from clients config
				for _, client := range configFile.Clients {
					if client.BandwidthLimit != nil {
						if bandwidthLimits.ByClientId == nil {
							bandwidthLimits.ByClientId = make(map[string]*limiter.BandwidthLimit)
						}
						bandwidthLimits.ByClientId[client.ClientID] = client.BandwidthLimit
					}
				}
			}

			// Use config reference for dynamic updates
			configRef := &struct {
				mu     sync.RWMutex
				config *ServerConfig
			}{
				config: configFile,
			}

			// Create GetToken function with config reference
			getToken := createGetTokenFunctionWithRef(configRef)

			// Create server options
			options := server.Options{
				Version:         ServerVersion,
				Port:            serverPort,
				TCPPort:         serverTCPPort,
				Secure:          serverSecure,
				Token:           getToken,
				Notification:    notificationConfig,
				BandwidthLimits: bandwidthLimits,
			}

			// Create and start server
			srv, err := server.New(options)
			if err != nil {
				return fmt.Errorf("failed to create server: %v", err)
			}

			// Setup config file watcher (only if config file exists)
			var watcher *fsnotify.Watcher
			if configPath != "" {
				if _, err := os.Stat(configPath); err == nil {
					watcher, err = fsnotify.NewWatcher()
					if err != nil {
						logger.Infof("[server:config] Warning: Failed to create file watcher: %v", err)
					} else {
						if err := watcher.Add(configPath); err != nil {
							logger.Infof("[server:config] Warning: Failed to watch config file: %v", err)
							watcher.Close()
							watcher = nil
						} else {
							logger.Infof("[server:config] Watching config file: %s", configPath)
						}
					}
				}
			}

			// Setup signal handling
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

			// Start server in a goroutine
			go func() {
				if err := srv.Start(); err != nil {
					fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
					os.Exit(1)
				}
			}()

			// Start config file watcher in a goroutine
			if watcher != nil {
				go watchConfigFile(watcher, configPath, configRef, srv)
			}

			// Wait for interrupt signal
			<-sigChan
			fmt.Println("\nShutting down server...")

			// Close watcher
			if watcher != nil {
				watcher.Close()
			}

			// Stop server
			if err := srv.Stop(); err != nil {
				return fmt.Errorf("failed to stop server: %v", err)
			}

			return nil
		},
	}
}

// findConfigFile finds configuration file by priority order
func findConfigFile() string {
	// Get current working directory and home directory
	wd, _ := os.Getwd()
	homeDir, _ := os.UserHomeDir()

	// Iterate through config file paths in priority order
	for _, pathTemplate := range ConfigFilePaths {
		// Resolve placeholders
		path := pathTemplate

		// Skip paths with {CWD} if we can't get working directory
		if strings.Contains(path, "{CWD}") {
			if wd == "" {
				continue
			}
			path = strings.ReplaceAll(path, "{CWD}", wd)
		}

		// Replace {HOME} placeholder
		if strings.Contains(path, "{HOME}") {
			if homeDir == "" {
				continue
			}
			path = strings.ReplaceAll(path, "{HOME}", homeDir)
		}

		// Convert to OS-specific path separators and normalize
		path = filepath.FromSlash(path)
		path = filepath.Clean(path)

		// Check if file exists
		if fileExists(path) {
			return path
		}
	}

	// No config file found
	return ""
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// loadConfigFile loads configuration from a YAML file
func loadConfigFile(path string) (*ServerConfig, error) {
	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, nil // File doesn't exist, not an error
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config ServerConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &config, nil
}

// createGetTokenFunctionWithRef creates a GetToken function that uses a config reference for dynamic updates
func createGetTokenFunctionWithRef(configRef *struct {
	mu     sync.RWMutex
	config *ServerConfig
}) types.GetToken {
	return func(authType types.AuthType, clientId string, options *types.GetTokenOptions) (*types.TokenResponse, error) {
		// Handle public auth
		if authType == types.AuthTypePublic {
			if options != nil && options.Type != types.TunnelTypeHTTP {
				return nil, fmt.Errorf("public auth is only allowed for http tunnel")
			}
			return &types.TokenResponse{
				AuthType: types.AuthTypePublic,
				Token:    "public",
			}, nil
		}

		// Get current config (with read lock)
		// Keep lock during entire operation to ensure consistency
		configRef.mu.RLock()
		defer configRef.mu.RUnlock()

		configFile := configRef.config
		if configFile == nil {
			return nil, fmt.Errorf("config file is required")
		}

		// Handle credentials auth
		if authType == types.AuthTypeCredentials {
			for _, clientCfg := range configFile.Clients {
				if clientCfg.ClientID == clientId {
					// Use client config if provided, otherwise create default config with version 2.0.0
					clientConfig := clientCfg.Config
					if clientConfig == nil {
						clientConfig = &client.Config{
							Version: ServerVersion,
						}
					} else if clientConfig.Version == "" {
						// If config exists but version is empty, create new config with default version
						clientConfig = &client.Config{
							Version:                ServerVersion,
							Notification:           clientConfig.Notification,
							NegotiatedCapabilities: clientConfig.NegotiatedCapabilities,
						}
					}
					return &types.TokenResponse{
						AuthType: types.AuthTypeCredentials,
						Token:    clientCfg.ClientSecret,
						Config:   clientConfig,
					}, nil
				}
			}
			return nil, fmt.Errorf("client not found: %s", clientId)
		}

		// Handle token auth (default)
		// Use token from config file if available, otherwise use provided token
		configToken := configFile.Token
		if configToken == "" {
			return nil, fmt.Errorf("token is required for token authentication")
		}

		return &types.TokenResponse{
			AuthType: types.AuthTypeToken,
			Token:    configToken,
			Config: &client.Config{
				Version: ServerVersion,
			},
		}, nil
	}
}

// watchConfigFile watches the config file for changes and reloads it
func watchConfigFile(watcher *fsnotify.Watcher, configPath string, configRef *struct {
	mu     sync.RWMutex
	config *ServerConfig
}, srv *server.Server) {
	var reloadTimer *time.Timer
	reloadDelay := 200 * time.Millisecond // Debounce delay
	defer func() {
		// Clean up timer on exit
		if reloadTimer != nil {
			reloadTimer.Stop()
		}
	}()

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Write == fsnotify.Write {
				logger.Infof("[server:config] Config file changed detected: %s", event.Name)

				// Cancel previous timer if exists
				if reloadTimer != nil {
					reloadTimer.Stop()
				}

				// Set new timer for debouncing
				reloadTimer = time.AfterFunc(reloadDelay, func() {
					reloadConfig(configPath, configRef, srv)
				})
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			logger.Infof("[server:config] File watcher error: %v", err)
		}
	}
}

// reloadConfig reloads the configuration file and updates the server
func reloadConfig(configPath string, configRef *struct {
	mu     sync.RWMutex
	config *ServerConfig
}, srv *server.Server) {
	// Get old clients count
	configRef.mu.RLock()
	oldClientsCount := 0
	if configRef.config != nil {
		oldClientsCount = len(configRef.config.Clients)
	}
	configRef.mu.RUnlock()

	// Load new config
	newConfig, err := loadConfigFile(configPath)
	if err != nil {
		logger.Infof("[server:config] Hot reload failed: %v", err)
		return
	}

	// Validate that Clients is not empty
	if newConfig == nil || len(newConfig.Clients) == 0 {
		logger.Infof("[server:config] Hot reload failed: clients configuration in config file %s cannot be empty", configPath)
		return
	}

	// Update config reference
	configRef.mu.Lock()
	configRef.config = newConfig
	configRef.mu.Unlock()

	// Get new clients count (newConfig is guaranteed to be non-nil at this point)
	newClientsCount := len(newConfig.Clients)

	logger.Infof("[server:config] Config file hot reloaded (client count: %d -> %d)", oldClientsCount, newClientsCount)

	// Build bandwidth limits from new config
	var bandwidthLimits *limiter.ClientBandwidthLimits
	if newConfig.BandwidthLimits != nil {
		bandwidthLimits = &limiter.ClientBandwidthLimits{
			ByClientId: make(map[string]*limiter.BandwidthLimit),
		}
		if newConfig.BandwidthLimits.Global != nil {
			bandwidthLimits.Global = newConfig.BandwidthLimits.Global
		}
		if newConfig.BandwidthLimits.Clients != nil {
			bandwidthLimits.ByClientId = newConfig.BandwidthLimits.Clients
		}
		// Also add bandwidth limits from clients config
		for _, client := range newConfig.Clients {
			if client.BandwidthLimit != nil {
				if bandwidthLimits.ByClientId == nil {
					bandwidthLimits.ByClientId = make(map[string]*limiter.BandwidthLimit)
				}
				bandwidthLimits.ByClientId[client.ClientID] = client.BandwidthLimit
			}
		}
	}

	// Create new GetToken function with updated config
	getToken := createGetTokenFunctionWithRef(configRef)

	// Setup notification config
	var notificationConfig *client.NotificationConfig
	if newConfig.Notification != nil {
		notificationConfig = newConfig.Notification
	}

	// Update server configuration
	if err := srv.UpdateConfig(getToken, notificationConfig, bandwidthLimits); err != nil {
		logger.Infof("[server:config] Failed to update server configuration: %v", err)
		return
	}

	// Send notification if configured
	if notificationConfig != nil {
		now := time.Now().Format("2006-01-02 15:04:05")
		title := "[Config Update] Config file reloaded"
		message := []string{
			fmt.Sprintf("Config file path: %s", configPath),
			fmt.Sprintf("Client count: %d -> %d", oldClientsCount, newClientsCount),
			fmt.Sprintf("Current time: %s", now),
		}
		// Note: We can't directly access the notification instance here,
		// but the UpdateConfig method should have updated it
		logger.Infof("[server:config] %s", title)
		for _, msg := range message {
			logger.Infof("[server:config]   %s", msg)
		}
	}
}
