package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/go-idp/inlets/internal/client"
	"github.com/go-idp/inlets/internal/server"
	"github.com/go-idp/inlets/internal/server/config"
	"github.com/go-zoox/logger"
	"github.com/urfave/cli/v2"
)

const ServerVersion = "2.0.0"

func Server() *cli.Command {
	flags := serverFlags()
	return &cli.Command{
		Name:  "server",
		Usage: "inlets tunnel server",
		Description: `Cloud Native Tunnel Server

  USAGE — server
  
    ▸ inlets server  <OPTIONS...>
    ▸ inlets server reload [-c config.yaml]`,
		Flags: flags,
		Subcommands: []*cli.Command{
			{
				Name:  "reload",
				Usage: "Reload server configuration (SIGHUP via pid file)",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "config",
						Aliases: []string{"c"},
						Usage:   "Config file path",
					},
				},
				Action: serverReloadAction,
			},
		},
		Action: serverRunAction,
	}
}

func serverFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:    "domain",
			Aliases: []string{"d"},
			Usage:   "Public tunnel domain (env: INLETS_DOMAIN)",
			EnvVars: []string{"INLETS_DOMAIN"},
		},
		&cli.IntFlag{
			Name:    "port",
			Aliases: []string{"p"},
			Usage:   "Port for server (default 8080) (env: INLETS_SERVER_PORT)",
			Value:   8080,
			EnvVars: []string{"INLETS_SERVER_PORT"},
		},
		&cli.BoolFlag{
			Name:    "secure",
			Aliases: []string{"s"},
			Usage:   "Use https in public tunnel URLs",
			Value:   false,
		},
		&cli.IntFlag{
			Name:    "tcp-port",
			Usage:   "TCP Port for server (default 8443) (env: INLETS_SERVER_TCP_PORT)",
			Value:   8443,
			EnvVars: []string{"INLETS_SERVER_TCP_PORT"},
		},
		&cli.StringFlag{
			Name:    "config",
			Aliases: []string{"c"},
			Usage:   "Config file path",
		},
		&cli.StringFlag{
			Name:    "notification-provider",
			Usage:   "Notification provider (env: INLETS_NOTIFICATION_PROVIDER)",
			EnvVars: []string{"INLETS_NOTIFICATION_PROVIDER"},
		},
		&cli.StringFlag{
			Name:    "notification-url",
			Usage:   "Notification webhook URL (env: INLETS_NOTIFICATION_URL)",
			EnvVars: []string{"INLETS_NOTIFICATION_URL"},
		},
	}
}

func resolveConfigPath(c *cli.Context) string {
	configPath := c.String("config")
	if configPath == "" {
		configPath = config.FindFile()
	}
	return configPath
}

func serverReloadAction(c *cli.Context) error {
	configPath := resolveConfigPath(c)
	if configPath == "" {
		return fmt.Errorf("config file not found; use -c /path/to/inlets.yaml")
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	pidFile := configPath + ".pid"
	if cfg != nil && cfg.Admin != nil {
		if resolved, err := config.ResolveAdmin(cfg, configPath); err == nil && resolved != nil && resolved.PidFile != "" {
			pidFile = resolved.PidFile
		}
	}
	if err := config.SignalReload(pidFile); err != nil {
		return fmt.Errorf("reload failed: %v", err)
	}
	fmt.Printf("Config reload signal sent (pid file: %s)\n", pidFile)
	return nil
}

func serverRunAction(c *cli.Context) error {
	configPath := resolveConfigPath(c)
	if configPath == "" {
		fmt.Println("No configuration file found. Please configure /etc/go-idp/inlets/config.yaml")
	}

	var configFile *config.FileConfig
	if configPath != "" {
		loaded, err := config.Load(configPath)
		if err != nil {
			logger.Infof("[server] Warning: Failed to load config file %s: %v", configPath, err)
			return fmt.Errorf("failed to load config file: %v", err)
		}
		if loaded != nil {
			if err := config.Validate(loaded); err != nil {
				return err
			}
			configFile = loaded
			logger.Infof("[server] Config file loaded: %s", configPath)
		}
	}

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

	var serverSecure bool
	if c.IsSet("secure") {
		serverSecure = c.Bool("secure")
	} else if sec := strings.TrimSpace(os.Getenv("INLETS_SECURE")); sec != "" {
		serverSecure = sec == "true" || sec == "1" || strings.EqualFold(sec, "yes")
	} else if configFile != nil && configFile.Secure != nil {
		serverSecure = *configFile.Secure
	}

	notificationProvider := c.String("notification-provider")
	if notificationProvider == "" && configFile != nil && configFile.Notification != nil {
		notificationProvider = configFile.Notification.Provider
	}
	notificationURL := c.String("notification-url")
	if notificationURL == "" && configFile != nil && configFile.Notification != nil {
		notificationURL = configFile.Notification.URL
	}

	serverDomain := strings.TrimSpace(c.String("domain"))
	if serverDomain == "" && configFile != nil {
		serverDomain = strings.TrimSpace(configFile.Domain)
	}
	if serverDomain == "" {
		return fmt.Errorf("domain is required: set `domain` in the config file, or use --domain / -d, or set INLETS_DOMAIN")
	}

	var notificationConfig *client.NotificationConfig
	if notificationProvider != "" && notificationURL != "" {
		notificationConfig = &client.NotificationConfig{
			Provider: notificationProvider,
			URL:      notificationURL,
		}
	} else if configFile != nil && configFile.Notification != nil {
		notificationConfig = configFile.Notification
	}

	bandwidthLimits := config.BuildBandwidthLimits(configFile)
	configRef := config.NewRef(configFile)
	getToken := config.CreateGetToken(configRef, ServerVersion)
	publicHTTPNoAuthTTL, publicHTTPNoAuthWarnLead := config.ResolvePublicHTTPNoAuthTiming(configFile)

	adminResolved, err := config.ResolveAdmin(configFile, configPath)
	if err != nil {
		return err
	}

	pidFile := ""
	if adminResolved != nil && adminResolved.PidFile != "" {
		pidFile = adminResolved.PidFile
	} else if configPath != "" {
		pidFile = configPath + ".pid"
	}

	srv, err := server.New(server.Options{
		Version:                      ServerVersion,
		Domain:                       serverDomain,
		Port:                         serverPort,
		TCPPort:                      serverTCPPort,
		Secure:                       serverSecure,
		Token:                        getToken,
		Notification:                 notificationConfig,
		BandwidthLimits:              bandwidthLimits,
		PublicHTTPNoAuthSessionTTL:   publicHTTPNoAuthTTL,
		PublicHTTPNoAuthWarnLeadTime: publicHTTPNoAuthWarnLead,
		ConfigPath:                   configPath,
		Admin:                        adminResolved,
		PidFile:                      pidFile,
	})
	if err != nil {
		return fmt.Errorf("failed to create server: %v", err)
	}

	var reloadMgr *config.Manager
	reloadMgr = config.NewManager(configPath, configRef, func(cfg *config.FileConfig) error {
		// The Manager updates ref.Set(cfg) BEFORE invoking apply, so
		// reading EffectiveConfig() now gives us ref + override.
		effective := reloadMgr.EffectiveConfig()
		opts := config.BuildApplyOptions(effective, ServerVersion, config.CreateGetToken(configRef, ServerVersion))
		if err := srv.UpdateConfig(
			opts.GetToken,
			opts.Notification,
			opts.BandwidthLimits,
			opts.PublicHTTPNoAuthSessionTTL,
			opts.PublicHTTPNoAuthWarnLeadTime,
		); err != nil {
			return err
		}
		return srv.ReconcileAdmin(effective)
	})
	srv.SetReloadManager(reloadMgr)
	if srv.AdminServer() != nil {
		srv.AdminServer().SetReloadManager(reloadMgr)
		srv.AdminServer().SetOverride(reloadMgr.Override())
	}

	var watcher *fsnotify.Watcher
	if configPath != "" {
		if _, err := os.Stat(configPath); err == nil {
			watcher, err = fsnotify.NewWatcher()
			if err != nil {
				logger.Infof("[server:config] Warning: Failed to create file watcher: %v", err)
			} else if err := watcher.Add(configPath); err != nil {
				logger.Infof("[server:config] Warning: Failed to watch config file: %v", err)
				watcher.Close()
				watcher = nil
			} else {
				logger.Infof("[server:config] Watching config file: %s", configPath)
			}
		}
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

	go func() {
		if err := srv.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
			os.Exit(1)
		}
	}()

	if watcher != nil {
		go watchConfigFile(watcher, reloadMgr)
	}

	for {
		sig := <-sigChan
		if sig == syscall.SIGHUP {
			if err := reloadMgr.Reload(); err != nil {
				logger.Infof("[server:config] Hot reload failed: %v", err)
			}
			continue
		}
		fmt.Println("\nShutting down server...")
		if watcher != nil {
			watcher.Close()
		}
		if err := srv.Stop(); err != nil {
			return fmt.Errorf("failed to stop server: %v", err)
		}
		return nil
	}
}

func watchConfigFile(watcher *fsnotify.Watcher, reloadMgr *config.Manager) {
	var reloadTimer *time.Timer
	reloadDelay := 200 * time.Millisecond
	defer func() {
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
				if reloadTimer != nil {
					reloadTimer.Stop()
				}
				reloadTimer = time.AfterFunc(reloadDelay, func() {
					if err := reloadMgr.Reload(); err != nil {
						logger.Infof("[server:config] Hot reload failed: %v", err)
					}
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
