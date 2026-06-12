package config

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-idp/inlets/internal/client"
	"github.com/go-idp/inlets/internal/server/limiter"
	"github.com/go-idp/inlets/internal/server/types"
	"github.com/go-zoox/logger"
)

// ApplyOptions carries runtime fields derived from FileConfig.
type ApplyOptions struct {
	GetToken                     types.GetToken
	Notification                 *client.NotificationConfig
	BandwidthLimits              *limiter.ClientBandwidthLimits
	PublicHTTPNoAuthSessionTTL   time.Duration
	PublicHTTPNoAuthWarnLeadTime time.Duration
}

// BuildApplyOptions constructs ApplyOptions from a config document.
func BuildApplyOptions(cfg *FileConfig, serverVersion string, getToken types.GetToken) ApplyOptions {
	if getToken == nil {
		getToken = CreateGetToken(NewRef(cfg), serverVersion)
	}
	ttl, warn := ResolvePublicHTTPNoAuthTiming(cfg)
	var notification *client.NotificationConfig
	if cfg != nil && cfg.Notification != nil {
		notification = cfg.Notification
	}
	return ApplyOptions{
		GetToken:                     getToken,
		Notification:                 notification,
		BandwidthLimits:              BuildBandwidthLimits(cfg),
		PublicHTTPNoAuthSessionTTL:   ttl,
		PublicHTTPNoAuthWarnLeadTime: warn,
	}
}

// ReloadFunc applies a freshly loaded config to the running server.
type ReloadFunc func(cfg *FileConfig) error

// Manager coordinates config file reload.
type Manager struct {
	mu    sync.Mutex
	path  string
	ref   *Ref
	apply ReloadFunc
}

func NewManager(path string, ref *Ref, apply ReloadFunc) *Manager {
	return &Manager{path: path, ref: ref, apply: apply}
}

func (m *Manager) Path() string {
	return m.path
}

func (m *Manager) Ref() *Ref {
	return m.ref
}

// Reload reads the config file, validates, updates ref, and applies.
func (m *Manager) Reload() error {
	if m == nil || m.path == "" {
		return fmt.Errorf("reload manager: config path not set")
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg, err := Load(m.path)
	if err != nil {
		return err
	}
	if err := Validate(cfg); err != nil {
		return err
	}
	m.ref.Set(cfg)
	if m.apply != nil {
		if err := m.apply(cfg); err != nil {
			return err
		}
	}
	logger.Infof("[server:config] Config reloaded from %s", m.path)
	return nil
}

func ResolvePublicHTTPNoAuthTiming(cfg *FileConfig) (time.Duration, time.Duration) {
	if cfg == nil || cfg.PublicHTTPNoAuth == nil {
		return 0, 0
	}
	var ttl, warn time.Duration
	if v := strings.TrimSpace(cfg.PublicHTTPNoAuth.Timeout); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			logger.Infof("[server] Warning: invalid publicHTTPNoAuth.timeout %q: %v", v, err)
		} else {
			ttl = d
		}
	}
	if v := strings.TrimSpace(cfg.PublicHTTPNoAuth.WarnLead); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			logger.Infof("[server] Warning: invalid publicHTTPNoAuth.warnLead %q: %v", v, err)
		} else {
			warn = d
		}
	}
	return ttl, warn
}

func BuildBandwidthLimits(cfg *FileConfig) *limiter.ClientBandwidthLimits {
	if cfg == nil {
		return nil
	}
	out := &limiter.ClientBandwidthLimits{ByClientId: make(map[string]*limiter.BandwidthLimit)}
	if cfg.BandwidthLimits != nil {
		if cfg.BandwidthLimits.Global != nil {
			out.Global = cfg.BandwidthLimits.Global
		}
		if cfg.BandwidthLimits.Clients != nil {
			out.ByClientId = cfg.BandwidthLimits.Clients
		}
	}
	for _, clientCfg := range cfg.Clients {
		if clientCfg.BandwidthLimit != nil {
			if out.ByClientId == nil {
				out.ByClientId = make(map[string]*limiter.BandwidthLimit)
			}
			out.ByClientId[clientCfg.ClientID] = clientCfg.BandwidthLimit
		}
	}
	if out.Global == nil && len(out.ByClientId) == 0 {
		return nil
	}
	return out
}
