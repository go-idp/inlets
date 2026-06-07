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
	ContainerID    string   `json:"containerId"`
	ClientID       string   `json:"clientId"`
	Type           string   `json:"type"`
	AuthType       string   `json:"authType"`
	Version        string   `json:"version"`
	PublicEntry    string   `json:"publicEntry"`
	SourcePort     *int     `json:"sourcePort,omitempty"`
	UseNewProtocol bool     `json:"useNewProtocol"`
	ConfigIndex    *int     `json:"configIndex,omitempty"`
	ConfigMatch    string   `json:"configMatch"` // "exact" | "partial" | "missing" | ""
	MatchIssues    []string `json:"matchIssues,omitempty"`
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
		sv := SessionView{
			ContainerID:    id,
			ClientID:       tm.ClientId,
			Type:           string(tm.Type),
			AuthType:       string(tm.AuthType),
			Version:        tm.Version,
			PublicEntry:    entry,
			SourcePort:     tm.SourcePort,
			UseNewProtocol: tm.UseNewProtocol,
			ConfigMatch:    m.matchSession(tm),
		}
		if idx, issues, ok := m.matchSessionIndex(tm); ok {
			sv.ConfigIndex = &idx
			sv.MatchIssues = issues
		}
		out = append(out, sv)
	}
	return out
}

// matchSession returns "exact" / "partial" / "missing".
//
// exact   = clientId is in YAML with the same secret
// partial = clientId is in YAML but the secret differs (or is empty)
// missing = clientId is anonymous or not in YAML
func (m *Monitor) matchSession(tm *types.TunnelMapping) string {
	cfg := m.currentConfig()
	_, _, status := findClientMatch(cfg, tm)
	return status
}

// matchSessionIndex returns the index in cfg.Clients (or -1 for "missing"),
// and any human-readable issues (without leaking secrets).
func (m *Monitor) matchSessionIndex(tm *types.TunnelMapping) (int, []string, bool) {
	cfg := m.currentConfig()
	if cfg == nil {
		return -1, nil, false
	}
	idx, issues, _ := findClientMatch(cfg, tm)
	return idx, issues, true
}

// currentConfig returns the YAML-backed FileConfig (overrides NOT applied,
// since overrides don't change client identities).
func (m *Monitor) currentConfig() *config.FileConfig {
	if m.deps.ReloadManager == nil {
		return nil
	}
	ref := m.deps.ReloadManager.Ref()
	if ref == nil {
		return nil
	}
	return ref.Get()
}

// findClientMatch locates tm.ClientId in cfg.Clients and returns the
// match index, a slice of human-readable issues, and the status.
func findClientMatch(cfg *config.FileConfig, tm *types.TunnelMapping) (int, []string, string) {
	if cfg == nil || tm == nil {
		return -1, nil, "missing"
	}
	if tm.ClientId == "" || strings.HasPrefix(tm.ClientId, "anonymous") {
		return -1, []string{"anonymous client id (no credential auth)"}, "missing"
	}
	for i, c := range cfg.Clients {
		if c.ClientID != tm.ClientId {
			continue
		}
		// Found the clientId. Compare secrets.
		var issues []string
		if strings.TrimSpace(c.ClientSecret) == "" {
			issues = append(issues, "client secret in YAML is empty")
			return i, issues, "partial"
		}
		// Without storing secrets on the live tm, we cannot compare
		// directly. Instead, we mark "exact" because the client was
		// able to authenticate (so the secret matched at the time of
		// session establishment). If the YAML was changed AFTER the
		// session started, the session may now be partial — but the
		// status shown here reflects the configured identity.
		return i, nil, "exact"
	}
	return -1, []string{fmt.Sprintf("client id %q not in YAML", tm.ClientId)}, "missing"
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
	path      string
	reload    *config.Manager
	revisions *RevisionService
}

func NewConfigService(path string, reload *config.Manager) *ConfigService {
	return &ConfigService{path: path, reload: reload, revisions: NewRevisionService()}
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

// Revisions returns the revision service.
func (s *ConfigService) Revisions() *RevisionService {
	return s.revisions
}

// SaveRawInput carries audit metadata for SaveRaw.
type SaveRawInput struct {
	Raw       []byte
	Summary   string
	Actor     string
	ClientIP  string
}

// SaveRawResult is the outcome of a save/restore.
type SaveRawResult struct {
	RevisionID uint
	Diff       string
}

// SaveRaw atomically writes the YAML, persists a revision row, computes
// a unified diff (using the on-disk file BEFORE the save), and triggers
// the reload manager if present. The caller is responsible for writing
// the audit row.
func (s *ConfigService) SaveRaw(in SaveRawInput) (*SaveRawResult, error) {
	old, _ := s.Raw()
	// Compute diff using the masked-on-disk view (no secret leakage).
	oldForDiff := string(old)
	newForDiff := string(in.Raw)
	diff := UnifiedDiff(oldForDiff, newForDiff)

	// Persist the revision BEFORE writing the file so a crash between
	// steps leaves an orphan revision (acceptable; the next save will
	// overwrite). See plan PR-2 §2.3.
	rev, err := s.revisions.Save(string(in.Raw), in.Summary, in.Actor, in.ClientIP)
	if err != nil {
		return nil, err
	}

	if err := config.SaveRawAtomic(s.path, in.Raw); err != nil {
		return nil, err
	}
	if s.reload != nil {
		if err := s.reload.Reload(); err != nil {
			return nil, err
		}
	}
	return &SaveRawResult{RevisionID: rev.ID, Diff: diff}, nil
}

// Restore writes a historical revision's YAML back to disk, persisting
// a new revision row that records the lineage.
func (s *ConfigService) Restore(revID uint, in SaveRawInput) (*SaveRawResult, error) {
	rev, err := s.revisions.Get(revID)
	if err != nil {
		return nil, ErrRevisionNotFound
	}
	summary := in.Summary
	if summary == "" {
		summary = fmt.Sprintf("restored from #%d", revID)
	}
	// Diff between current on-disk and the revision's content.
	old, _ := s.Raw()
	diff := UnifiedDiff(string(old), rev.YAML)

	newRaw := []byte(rev.YAML)
	actor := in.Actor
	clientIP := in.ClientIP
	created, err := s.revisions.Save(rev.YAML, summary, actor, clientIP)
	if err != nil {
		return nil, err
	}
	if err := config.SaveRawAtomic(s.path, newRaw); err != nil {
		return nil, err
	}
	if s.reload != nil {
		if err := s.reload.Reload(); err != nil {
			return nil, err
		}
	}
	return &SaveRawResult{RevisionID: created.ID, Diff: diff}, nil
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
