package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-idp/inlets/internal/server/config"
	"github.com/go-idp/inlets/internal/server/stats"
	"github.com/go-idp/inlets/internal/server/types"
)

// Monitor provides read-only views of runtime state.
type Monitor struct {
	deps RuntimeDeps
}

func NewMonitor(deps RuntimeDeps) *Monitor {
	return &Monitor{deps: deps}
}

func (m *Monitor) Overview() map[string]any {
	sessions := m.Sessions()
	return map[string]any{
		"version":       m.deps.ServerVersion,
		"domain":        m.deps.Domain,
		"httpPort":      m.deps.HTTPPort,
		"tcpPort":       m.deps.TCPPort,
		"secure":        m.deps.Secure,
		"uptimeSeconds": int64(time.Since(m.deps.Started).Seconds()),
		"startedAt":     m.deps.Started.Format(time.RFC3339),
		"sessionCount":  len(sessions),
		"domainCount":   len(m.Domains()),
		"stats":         m.StatsGlobal(),
	}
}

type SessionView struct {
	ContainerID    string `json:"containerId"`
	ClientID       string `json:"clientId"`
	Type           string `json:"type"`
	AuthType       string `json:"authType"`
	Version        string `json:"version"`
	PublicEntry    string `json:"publicEntry"`
	SourcePort     *int   `json:"sourcePort,omitempty"`
	UseNewProtocol bool   `json:"useNewProtocol"`
}

func (m *Monitor) Sessions() []SessionView {
	if m.deps.Ctx == nil || m.deps.Ctx.Container == nil {
		return nil
	}
	domainByContainer := map[string]string{}
	if m.deps.Ctx.DomainMappings != nil {
		for sub, dm := range m.deps.Ctx.DomainMappings.GetAll() {
			if dm == nil || dm.ContainerID == "" {
				continue
			}
			scheme := "http"
			if m.deps.Secure {
				scheme = "https"
			}
			domainByContainer[dm.ContainerID] = fmt.Sprintf("%s://%s.%s", scheme, sub, m.deps.Domain)
		}
	}
	all := m.deps.Ctx.Container.ListAll()
	out := make([]SessionView, 0, len(all))
	for id, tm := range all {
		if tm == nil {
			continue
		}
		entry := domainByContainer[id]
		if entry == "" {
			entry = m.publicEntry(tm)
		}
		out = append(out, SessionView{
			ContainerID:    id,
			ClientID:       tm.ClientId,
			Type:           string(tm.Type),
			AuthType:       string(tm.AuthType),
			Version:        tm.Version,
			PublicEntry:    entry,
			SourcePort:     tm.SourcePort,
			UseNewProtocol: tm.UseNewProtocol,
		})
	}
	return out
}

func (m *Monitor) publicEntry(tm *types.TunnelMapping) string {
	if tm.Type == types.TunnelTypeHTTP {
		scheme := "http"
		if m.deps.Secure {
			scheme = "https"
		}
		return fmt.Sprintf("%s://*.%s", scheme, m.deps.Domain)
	}
	if tm.SourcePort != nil {
		return fmt.Sprintf("0.0.0.0:%d", *tm.SourcePort)
	}
	return ""
}

type DomainView struct {
	SubDomain string `json:"subDomain"`
	ClientID  string `json:"clientId"`
}

func (m *Monitor) Domains() []DomainView {
	if m.deps.Ctx == nil || m.deps.Ctx.DomainMappings == nil {
		return nil
	}
	all := m.deps.Ctx.DomainMappings.GetAll()
	out := make([]DomainView, 0, len(all))
	for sub, dm := range all {
		if dm == nil {
			continue
		}
		out = append(out, DomainView{SubDomain: sub, ClientID: dm.ClientID})
	}
	return out
}

func (m *Monitor) StatsGlobal() map[string]any {
	if m.deps.Ctx == nil || m.deps.Ctx.TrafficStats == nil {
		return nil
	}
	raw := m.deps.Ctx.TrafficStats.GetStats("")
	data, ok := raw.(*stats.TrafficStatsData)
	if !ok || data == nil || data.Global == nil {
		return nil
	}
	g := data.Global
	return map[string]any{
		"uploadBytes":   g.UploadBytes,
		"downloadBytes": g.DownloadBytes,
		"requests":      g.Requests,
		"connections":   g.Connections,
	}
}

func (m *Monitor) StatsByClient() map[string]map[string]any {
	if m.deps.Ctx == nil || m.deps.Ctx.TrafficStats == nil {
		return nil
	}
	raw := m.deps.Ctx.TrafficStats.GetStats("")
	data, ok := raw.(*stats.TrafficStatsData)
	if !ok || data == nil {
		return nil
	}
	out := make(map[string]map[string]any)
	for clientID, st := range data.ByClientId {
		if st == nil {
			continue
		}
		out[clientID] = map[string]any{
			"uploadBytes":   st.UploadBytes,
			"downloadBytes": st.DownloadBytes,
			"requests":      st.Requests,
			"connections":   st.Connections,
		}
	}
	return out
}

// ConfigService reads/writes the server YAML file.
type ConfigService struct {
	path   string
	reload *config.Manager
}

func NewConfigService(path string, reload *config.Manager) *ConfigService {
	return &ConfigService{path: path, reload: reload}
}

func (s *ConfigService) Path() string {
	return s.path
}

func (s *ConfigService) Raw() ([]byte, error) {
	return config.LoadRaw(s.path)
}

func (s *ConfigService) Document(maskSecrets bool) (*config.FileConfig, error) {
	cfg, err := config.Load(s.path)
	if err != nil {
		return nil, err
	}
	if maskSecrets {
		maskConfigSecrets(cfg)
	}
	return cfg, nil
}

func (s *ConfigService) SaveRaw(raw []byte) error {
	if err := config.SaveRawAtomic(s.path, raw); err != nil {
		return err
	}
	if s.reload != nil {
		return s.reload.Reload()
	}
	return nil
}

func maskConfigSecrets(cfg *config.FileConfig) {
	if cfg == nil {
		return
	}
	if cfg.Token != "" {
		cfg.Token = "***"
	}
	for i := range cfg.Clients {
		if cfg.Clients[i].ClientSecret != "" {
			cfg.Clients[i].ClientSecret = "***"
		}
	}
}

func ShortContainerID(id string) string {
	id = strings.TrimSpace(id)
	if len(id) <= 12 {
		return id
	}
	return id[:4] + "…" + id[len(id)-4:]
}
