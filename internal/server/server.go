package server

import (
	"fmt"
	"net"
	"net/http"
	"time"

	inlets "github.com/go-idp/inlets"
	"github.com/go-idp/inlets/internal/client"
	datachannel "github.com/go-idp/inlets/internal/server/channels/data"
	monitorchannel "github.com/go-idp/inlets/internal/server/channels/monitor"
	servercontainer "github.com/go-idp/inlets/internal/server/container"
	"github.com/go-idp/inlets/internal/server/admin"
	"github.com/go-idp/inlets/internal/server/config"
	"github.com/go-idp/inlets/internal/server/limiter"
	"github.com/go-idp/inlets/internal/server/stats"
	"github.com/go-idp/inlets/internal/server/tunnel"
	"github.com/go-idp/inlets/internal/server/types"
	"github.com/go-idp/inlets/internal/server/utils"
	"github.com/go-zoox/logger"
)

// Options contains options for creating a server
type Options struct {
	Version         string
	Domain          string
	Port            int
	TCPPort         int
	Secure          bool
	Token           types.GetToken
	Notification    *client.NotificationConfig
	BandwidthLimits *limiter.ClientBandwidthLimits
	// PublicHTTPNoAuthSessionTTL: max lifetime for public (unauthenticated) monitor clients.
	// Zero means default 10m. Independent of tunnel type and public URL auth.
	PublicHTTPNoAuthSessionTTL time.Duration
	// PublicHTTPNoAuthWarnLeadTime: lead time before close warning.
	// Zero means default 2m.
	PublicHTTPNoAuthWarnLeadTime time.Duration
	ConfigPath    string
	ReloadManager *config.Manager
	Admin         *config.ResolvedAdmin
	PidFile       string
}

// Server represents the main server instance
type Server struct {
	ctx          *types.Context
	httpServer   *http.Server
	wsMonitor    *WebSocketMonitor
	tcpMonitor   *datachannel.TCPMonitor
	httpTunnel   *tunnel.HTTPTunnel
	tcpTunnel    *tunnel.TCPTunnel
	notification *monitorchannel.Notification
	adminServer  *admin.Server
	startedAt    time.Time
	options      Options
}

// WebSocketMonitor manages WebSocket connections and authentication
type WebSocketMonitor struct {
	ctx            *types.Context
	options        *monitorchannel.CreateWebSocketOptions
	monitorHandler *monitorchannel.MonitorChannelHandler
	dataHandler    *datachannel.WebSocketDataChannelHandler
	emitter        *monitorchannel.EventEmitter
}

// CreateWebSocketMonitor creates a new WebSocket monitor
func CreateWebSocketMonitor(ctx *types.Context, options *monitorchannel.CreateWebSocketOptions) *WebSocketMonitor {
	emitter := monitorchannel.NewEventEmitter()
	monitorHandler := monitorchannel.NewMonitorChannelHandler(ctx, options, emitter)
	dataHandler := datachannel.NewDataChannelHandler(ctx)

	return &WebSocketMonitor{
		ctx:            ctx,
		options:        options,
		monitorHandler: monitorHandler,
		dataHandler:    dataHandler,
		emitter:        emitter,
	}
}

// Attach attaches the WebSocket monitor to an HTTP server
func (m *WebSocketMonitor) Attach(server *http.Server) {
	mux := tunnel.ServeMuxFor(server)
	if mux == nil {
		logger.Infof("[server] WebSocket Attach: server.Handler is %T, not *http.ServeMux; WebSocket routes not registered", server.Handler)
		return
	}

	// Legacy protocol: single connection at /_client (handles all messages)
	mux.HandleFunc(m.ctx.Config.WSPath, m.monitorHandler.HandleConnectionLegacy)

	// New protocol: separated channels
	// Monitor channel: ping/pong, auth, control messages
	mux.HandleFunc(m.ctx.Config.WSMonitorPath, m.monitorHandler.HandleConnection)
	// Data channel: tcp:data only
	mux.HandleFunc(m.ctx.Config.WSDataPath, m.dataHandler.HandleConnection)
}

// OnTunnel registers a handler for tunnel events
func (m *WebSocketMonitor) OnTunnel(handler func(data map[string]interface{})) {
	m.emitter.On("tunnel", func(data interface{}) {
		if dataMap, ok := data.(map[string]interface{}); ok {
			handler(dataMap)
		}
	})
}

// New creates and initializes a new server instance
func New(options Options) (*Server, error) {
	// Get or allocate ports
	port := options.Port
	if port == 0 {
		var err error
		port, err = utils.GetAvailablePort()
		if err != nil {
			return nil, fmt.Errorf("failed to allocate HTTP port: %v", err)
		}
	}

	tcpPort := options.TCPPort
	if tcpPort == 0 {
		var err error
		tcpPort, err = utils.GetAvailablePort()
		if err != nil {
			return nil, fmt.Errorf("failed to allocate TCP port: %v", err)
		}
	}

	// Initialize Context
	ctx := &types.Context{
		Config:            types.DefaultServerConfig(),
		DomainMappings:    servercontainer.NewDomainContainer(),
		CallbackContainer: servercontainer.NewCallbackContainer(),
		Container:         servercontainer.NewTunnelContainer(),
		TrafficStats:      stats.NewTrafficStatsContainer(),
		BandwidthLimiter:  limiter.NewBandwidthLimiter(options.BandwidthLimits),
	}
	if options.Version != "" {
		ctx.Config.Version = options.Version
	}

	// Create notification service
	notification := monitorchannel.NewNotification(options.Notification)

	// Create WebSocket monitor
	wsMonitor := CreateWebSocketMonitor(ctx, &monitorchannel.CreateWebSocketOptions{
		Version:                     options.Version,
		Domain:                      options.Domain,
		Port:                        port,
		Secure:                      options.Secure,
		Token:                       options.Token,
		Notification:                notification,
		PublicHTTPNoAuthSessionTTL:  options.PublicHTTPNoAuthSessionTTL,
		PublicHTTPNoAuthWarnLeadTime: options.PublicHTTPNoAuthWarnLeadTime,
	})

	// Create TCP monitor
	tcpMonitor, err := datachannel.NewDataChannelHandlerLegacy(ctx, &datachannel.CreateTCPMonitorOptions{
		Version: options.Version,
		Domain:  options.Domain,
		Port:    tcpPort,
		Token:   options.Token,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create TCP monitor: %v", err)
	}

	// Create HTTP tunnel
	httpTunnel := tunnel.CreateHTTPTunnel(ctx, options.Domain)

	// Create TCP tunnel
	tcpTunnel := tunnel.CreateTCPTunnel(ctx)

	// Create HTTP server
	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: http.DefaultServeMux,
	}

	// IMPORTANT: Attach WebSocket monitor FIRST so it handles WebSocket upgrades
	// before HTTP tunnel intercepts connections
	wsMonitor.Attach(httpServer)

	// Attach HTTP tunnel to HTTP server (this adds a catch-all handler)
	// The HTTP tunnel handler will only process non-WebSocket requests
	httpTunnel.Attach(httpServer)

	// Add stats API endpoints
	utils.SetupStatsAPI(ctx)

	// Listen for tunnel events
	wsMonitor.OnTunnel(func(data map[string]interface{}) {
		tunnelType, ok := data["type"].(string)
		if !ok {
			return
		}

		containerID, ok := data["containerId"].(string)
		if !ok {
			return
		}

		logger.Infof("[server] Tunnel event received: type=%s, containerId=%s", tunnelType, containerID)

		if tunnelType == "tcp" {
			// Create TCP tunnel server
			logger.Infof("[server] Creating TCP tunnel server for container: %s", containerID)
			if err := tcpTunnel.CreateServer(tunnel.Options{
				ContainerID: containerID,
				Domain:      options.Domain,
			}); err != nil {
				logger.Infof("[server] Failed to create TCP tunnel: %v", err)
			} else {
				logger.Infof("[server] TCP tunnel server created successfully for container: %s", containerID)
			}
		} else if tunnelType == "http" {
			// HTTP tunnel doesn't need a separate listener - it uses the main HTTP server
			// The domain mapping is already set up in handleAuthenticate
			logger.Infof("[server] HTTP tunnel ready for container: %s (using main HTTP server on port %d)", containerID, port)
		}
	})

	server := &Server{
		ctx:          ctx,
		httpServer:   httpServer,
		wsMonitor:    wsMonitor,
		tcpMonitor:   tcpMonitor,
		httpTunnel:   httpTunnel,
		tcpTunnel:    tcpTunnel,
		notification: notification,
		options:      options,
	}

	if options.Admin != nil && options.Admin.Enabled {
		adminSrv, err := admin.New(admin.Options{
			Resolved:      options.Admin,
			ConfigPath:    options.ConfigPath,
			ReloadManager: options.ReloadManager,
			ServerVersion: options.Version,
			Domain:        options.Domain,
			HTTPPort:      port,
			TCPPort:       tcpPort,
			Secure:        options.Secure,
			Ctx:           ctx,
			ServerStarted: time.Now(),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create admin server: %v", err)
		}
		server.adminServer = adminSrv
	}

	return server, nil
}

// Start starts the server
func (s *Server) Start() error {
	s.startedAt = time.Now()
	if s.adminServer != nil {
		if err := s.adminServer.Start(); err != nil {
			return fmt.Errorf("failed to start admin server: %v", err)
		}
	}
	if s.options.PidFile != "" {
		if err := config.WritePIDFile(s.options.PidFile); err != nil {
			logger.Infof("[server] Warning: failed to write pid file: %v", err)
		}
	}
	// Get machine IP
	ip, err := utils.GetMachineIP()
	if err != nil {
		logger.Infof("[server] Failed to get machine IP: %v", err)
		ip = "未知"
	}

	// Start HTTP server in a goroutine
	go func() {
		logger.Infof("[server] Version: %s", inlets.Version)
		logger.Infof("[server] Starting HTTP server on port %s", s.httpServer.Addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Infof("[server] HTTP server error: %v", err)
		} else {
			logger.Infof("[server] HTTP server stopped")
		}
	}()

	// Give the server a moment to start
	time.Sleep(100 * time.Millisecond)

	// Verify server is reachable by dialing instead of binding again
	conn, err := net.DialTimeout("tcp", s.httpServer.Addr, 500*time.Millisecond)
	if err != nil {
		return fmt.Errorf("port %s not reachable after start: %v", s.httpServer.Addr, err)
	}
	conn.Close()
	logger.Infof("[server] Verified: Port %s is accepting connections", s.httpServer.Addr)

	// Send notification
	now := time.Now().Format("2006-01-02 15:04:05")
	title := fmt.Sprintf("[服务端][启动] 成功 - %s", ip)
	message := []string{
		fmt.Sprintf("服务端版本：%s", s.ctx.Config.Version),
		fmt.Sprintf("服务端IP: %s", ip),
		fmt.Sprintf("当前时间：%s", now),
	}
	if err := s.notification.Notify(title, message); err != nil {
		logger.Infof("[server] Failed to send notification: %v", err)
	}

	logger.Infof("[server] Server started successfully")
	logger.Infof("[server] HTTP server listening on %s", s.httpServer.Addr)
	logger.Infof("[server] TCP monitor listening on port %d", s.tcpMonitor.GetPort())

	return nil
}

// UpdateConfig updates the server configuration dynamically
func (s *Server) UpdateConfig(
	getToken types.GetToken,
	notificationConfig *client.NotificationConfig,
	bandwidthLimits *limiter.ClientBandwidthLimits,
	publicHTTPNoAuthSessionTTL time.Duration,
	publicHTTPNoAuthWarnLeadTime time.Duration,
) error {
	// Update GetToken function in WebSocket monitor
	if s.wsMonitor != nil && s.wsMonitor.options != nil {
		s.wsMonitor.options.Token = getToken
		s.wsMonitor.options.PublicHTTPNoAuthSessionTTL = publicHTTPNoAuthSessionTTL
		s.wsMonitor.options.PublicHTTPNoAuthWarnLeadTime = publicHTTPNoAuthWarnLeadTime
	}

	// Update notification instance
	if notificationConfig != nil {
		s.notification = monitorchannel.NewNotification(notificationConfig)
		// Update notification in WebSocket monitor options
		if s.wsMonitor != nil && s.wsMonitor.options != nil {
			s.wsMonitor.options.Notification = s.notification
		}
	} else {
		s.notification = nil
		if s.wsMonitor != nil && s.wsMonitor.options != nil {
			s.wsMonitor.options.Notification = nil
		}
	}

	// Update bandwidth limiter
	if s.ctx != nil && s.ctx.BandwidthLimiter != nil {
		if updater, ok := s.ctx.BandwidthLimiter.(interface {
			UpdateLimits(*limiter.ClientBandwidthLimits)
		}); ok {
			updater.UpdateLimits(bandwidthLimits)
		}
	}

	return nil
}

// ReconcileAdmin (re)starts the admin HTTP listener when admin.listen or related
// settings change. Hot reload updates tunnel auth and limits but admin binds its
// own socket at startup; without this, saving 0.0.0.0:9090 in the UI would not
// take effect until process restart.
func (s *Server) ReconcileAdmin(cfg *config.FileConfig) error {
	resolved, err := config.ResolveAdmin(cfg, s.options.ConfigPath)
	if err != nil {
		return err
	}

	wantEnabled := resolved != nil && resolved.Enabled
	if !wantEnabled {
		if s.adminServer != nil {
			if err := s.adminServer.Stop(); err != nil {
				return fmt.Errorf("stop admin server: %w", err)
			}
			s.adminServer = nil
		}
		s.options.Admin = nil
		return nil
	}

	if s.adminServer != nil && s.options.Admin != nil && s.options.Admin.SameListen(resolved) {
		s.options.Admin = resolved
		return nil
	}

	if s.adminServer != nil {
		if err := s.adminServer.Stop(); err != nil {
			logger.Infof("[admin] stop before rebind: %v", err)
		}
		s.adminServer = nil
	}

	adminSrv, err := admin.New(admin.Options{
		Resolved:      resolved,
		ConfigPath:    s.options.ConfigPath,
		ReloadManager: s.options.ReloadManager,
		ServerVersion: s.options.Version,
		Domain:        s.options.Domain,
		HTTPPort:      s.options.Port,
		TCPPort:       s.options.TCPPort,
		Secure:        s.options.Secure,
		Ctx:           s.ctx,
		ServerStarted: s.startedAt,
	})
	if err != nil {
		return fmt.Errorf("create admin server: %w", err)
	}
	if err := adminSrv.Start(); err != nil {
		return fmt.Errorf("start admin server: %w", err)
	}
	s.adminServer = adminSrv
	s.options.Admin = resolved
	logger.Infof("[admin] Admin listener reconfigured on http://%s:%d", resolved.Host, resolved.Port)
	return nil
}

// SetReloadManager attaches the hot-reload manager after construction.
func (s *Server) SetReloadManager(m *config.Manager) {
	s.options.ReloadManager = m
}

// AdminServer returns the admin console server when enabled.
func (s *Server) AdminServer() *admin.Server {
	return s.adminServer
}

func (s *Server) Stop() error {
	config.RemovePIDFile(s.options.PidFile)
	if s.adminServer != nil {
		if err := s.adminServer.Stop(); err != nil {
			logger.Infof("[server] Error stopping admin server: %v", err)
		}
	}
	// Close HTTP server
	if s.httpServer != nil {
		if err := s.httpServer.Close(); err != nil {
			logger.Infof("[server] Error closing HTTP server: %v", err)
		}
	}

	// Close TCP monitor
	if s.tcpMonitor != nil {
		if err := s.tcpMonitor.Close(); err != nil {
			logger.Infof("[server] Error closing TCP monitor: %v", err)
		}
	}

	return nil
}
