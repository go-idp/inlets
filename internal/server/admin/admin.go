package admin

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/go-idp/inlets/internal/server/admin/bootstrap"
	"github.com/go-idp/inlets/internal/server/admin/handler"
	"github.com/go-idp/inlets/internal/server/admin/service"
	"github.com/go-idp/inlets/internal/server/admin/static"
	"github.com/go-idp/inlets/internal/server/config"
	"github.com/go-idp/inlets/internal/server/types"
	"github.com/go-zoox/logger"
	"github.com/go-zoox/zoox/defaults"
)

// Options configures the admin HTTP server.
type Options struct {
	Resolved      *config.ResolvedAdmin
	ConfigPath    string
	ReloadManager *config.Manager
	ServerStarted time.Time
	ServerVersion string
	Domain        string
	HTTPPort      int
	TCPPort       int
	Secure        bool
	Ctx           *types.Context
}

// Server runs the admin HTTP API and UI.
type Server struct {
	opts     Options
	deps     service.RuntimeDeps
	httpSrv  *http.Server
	listener net.Listener
}

// New creates an admin server instance.
func New(opts Options) (*Server, error) {
	if opts.Resolved == nil || !opts.Resolved.Enabled {
		return nil, fmt.Errorf("admin is not enabled")
	}
	if err := bootstrap.Init(opts.Resolved.DatabasePath); err != nil {
		return nil, err
	}
	deps := service.RuntimeDeps{
		Ctx:           opts.Ctx,
		Domain:        opts.Domain,
		HTTPPort:      opts.HTTPPort,
		TCPPort:       opts.TCPPort,
		Secure:        opts.Secure,
		ServerVersion: opts.ServerVersion,
		Started:       opts.ServerStarted,
		ConfigPath:    opts.ConfigPath,
		ReloadManager: opts.ReloadManager,
	}
	return &Server{opts: opts, deps: deps}, nil
}

// Start listens and serves admin HTTP in the background.
func (s *Server) Start() error {
	if s.deps.Started.IsZero() {
		s.deps.Started = time.Now()
	}
	res := s.opts.Resolved
	app := defaults.Application()
	app.SetBanner("")
	app.Config.Host = res.Host
	app.Config.Port = res.Port

	api := handler.New(s.deps)
	g := app.Group("/api/v1")
	api.Mount(g)

	if err := static.Mount(app, res.UIBasePath); err != nil {
		logger.Infof("[admin] static UI not mounted: %v", err)
	}

	addr := net.JoinHostPort(res.Host, fmt.Sprintf("%d", res.Port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("admin listen %s: %w", addr, err)
	}
	s.listener = ln

	s.httpSrv = &http.Server{
		Handler:      app,
		ReadTimeout:  120 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		logger.Infof("[admin] Admin console listening on http://%s", addr)
		if err := s.httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
			logger.Infof("[admin] Admin server error: %v", err)
		}
	}()

	if res.SnapshotInterval > 0 {
		go s.runSnapshotLoop(res.SnapshotInterval)
	}
	return nil
}

func (s *Server) runSnapshotLoop(every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for range ticker.C {
		if err := handler.RecordSnapshot(s.deps.Ctx); err != nil {
			logger.Infof("[admin] metric snapshot failed: %v", err)
		}
	}
}

// Stop shuts down the admin HTTP server.
func (s *Server) Stop() error {
	if s.httpSrv == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.httpSrv.Shutdown(ctx)
}

// SetReloadManager updates reload manager after server wiring.
func (s *Server) SetReloadManager(m *config.Manager) {
	s.deps.ReloadManager = m
	s.opts.ReloadManager = m
}
