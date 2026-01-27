package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"

	"github.com/go-zoox/inlets/internal/client"
	"github.com/go-zoox/inlets/internal/server"
	"github.com/go-zoox/inlets/internal/server/limiter"
	"github.com/go-zoox/inlets/internal/server/types"
	"github.com/urfave/cli/v2"
)

const (
	ServerVersion = "2.0.0"
)

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
			&cli.StringFlag{
				Name:    "domain",
				Aliases: []string{"d"},
				Usage:   "Domain for server",
				EnvVars: []string{"DOMAIN"},
			},
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
				Name:    "token",
				Aliases: []string{"t"},
				Usage:   "Token for authentication",
				EnvVars: []string{"TOKEN"},
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
			// Get default config path
			defaultConfigPath := ""
			if homeDir, err := os.UserHomeDir(); err == nil {
				defaultConfigPath = filepath.Join(homeDir, ".config", "inlets.yml")
			}

			// Get config path
			configPath := c.String("config")
			if configPath == "" {
				configPath = defaultConfigPath
			}

			// Load config file if specified or default exists
			var configFile *ServerConfig
			if configPath != "" {
				loadedConfig, err := loadConfigFile(configPath)
				if err != nil {
					log.Printf("[server] Warning: Failed to load config file %s: %v", configPath, err)
				} else if loadedConfig != nil {
					configFile = loadedConfig
					log.Printf("[server] Config file loaded: %s", configPath)
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

			serverDomain := c.String("domain")
			if serverDomain == "" {
				if configFile != nil && configFile.Domain != "" {
					serverDomain = configFile.Domain
				}
			}

			serverToken := c.String("token")
			if serverToken == "" {
				if configFile != nil && configFile.Token != "" {
					serverToken = configFile.Token
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

			// Validate required options
			if serverDomain == "" {
				return fmt.Errorf("domain is required (use --domain, set DOMAIN env var, or specify in config file)")
			}

			// Validate token requirement
			if serverToken == "" && (configFile == nil || len(configFile.Clients) == 0) {
				return fmt.Errorf("token is required (use --token, set TOKEN env var, or specify in config file)")
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
			getToken := createGetTokenFunctionWithRef(serverToken, configRef)

			// Create server options
			options := server.Options{
				Version:         ServerVersion,
				Domain:          serverDomain,
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
						log.Printf("[server:config] Warning: Failed to create file watcher: %v", err)
					} else {
						if err := watcher.Add(configPath); err != nil {
							log.Printf("[server:config] Warning: Failed to watch config file: %v", err)
							watcher.Close()
							watcher = nil
						} else {
							log.Printf("[server:config] Watching config file: %s", configPath)
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
				go watchConfigFile(watcher, configPath, configRef, srv, serverToken)
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
func createGetTokenFunctionWithRef(token string, configRef *struct {
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
		configRef.mu.RLock()
		configFile := configRef.config
		configRef.mu.RUnlock()

		// Handle credentials auth
		if authType == types.AuthTypeCredentials {
			// Look up client secret from config file
			if configFile != nil && len(configFile.Clients) > 0 {
				for _, client := range configFile.Clients {
					if client.ClientID == clientId {
						return &types.TokenResponse{
							AuthType: types.AuthTypeCredentials,
							Token:    client.ClientSecret,
							Config:   client.Config,
						}, nil
					}
				}
				return nil, fmt.Errorf("client not found: %s", clientId)
			}
			// Fallback to token if no clients configured
			if token == "" {
				return nil, fmt.Errorf("credentials auth requires clients configuration or token")
			}
			return &types.TokenResponse{
				AuthType: types.AuthTypeCredentials,
				Token:    token,
			}, nil
		}

		// Handle token auth (default)
		// Use token from config file if available, otherwise use provided token
		configToken := token
		if configFile != nil && configFile.Token != "" {
			configToken = configFile.Token
		}
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
}, srv *server.Server, defaultToken string) {
	var reloadTimer *time.Timer
	reloadDelay := 200 * time.Millisecond // Debounce delay

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Write == fsnotify.Write {
				log.Printf("[server:config] 检测到配置文件变化: %s", event.Name)

				// Cancel previous timer if exists
				if reloadTimer != nil {
					reloadTimer.Stop()
				}

				// Set new timer for debouncing
				reloadTimer = time.AfterFunc(reloadDelay, func() {
					reloadConfig(configPath, configRef, srv, defaultToken)
				})
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("[server:config] 文件监听出错: %v", err)
		}
	}
}

// reloadConfig reloads the configuration file and updates the server
func reloadConfig(configPath string, configRef *struct {
	mu     sync.RWMutex
	config *ServerConfig
}, srv *server.Server, defaultToken string) {
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
		log.Printf("[server:config] 热更新失败: %v", err)
		return
	}

	// Update config reference
	configRef.mu.Lock()
	configRef.config = newConfig
	configRef.mu.Unlock()

	// Get new clients count
	newClientsCount := 0
	if newConfig != nil {
		newClientsCount = len(newConfig.Clients)
	}

	log.Printf("[server:config] 配置文件已热更新 (客户端数量: %d -> %d)", oldClientsCount, newClientsCount)

	// Build bandwidth limits from new config
	var bandwidthLimits *limiter.ClientBandwidthLimits
	if newConfig != nil && newConfig.BandwidthLimits != nil {
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
	getToken := createGetTokenFunctionWithRef(defaultToken, configRef)

	// Setup notification config
	var notificationConfig *client.NotificationConfig
	if newConfig != nil && newConfig.Notification != nil {
		notificationConfig = newConfig.Notification
	}

	// Update server configuration
	if err := srv.UpdateConfig(getToken, notificationConfig, bandwidthLimits); err != nil {
		log.Printf("[server:config] 更新服务器配置失败: %v", err)
		return
	}

	// Send notification if configured
	if notificationConfig != nil {
		now := time.Now().Format("2006-01-02 15:04:05")
		title := "[配置更新] 配置文件已重新加载"
		message := []string{
			fmt.Sprintf("配置文件路径: %s", configPath),
			fmt.Sprintf("客户端数量: %d -> %d", oldClientsCount, newClientsCount),
			fmt.Sprintf("当前时间: %s", now),
		}
		// Note: We can't directly access the notification instance here,
		// but the UpdateConfig method should have updated it
		log.Printf("[server:config] %s", title)
		for _, msg := range message {
			log.Printf("[server:config]   %s", msg)
		}
	}
}
