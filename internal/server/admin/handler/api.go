package handler

import (
	"io"
	"net/http"
	"strconv"

	"github.com/go-idp/inlets/internal/server/admin/model"
	"github.com/go-idp/inlets/internal/server/admin/service"
	"github.com/go-idp/inlets/internal/server/stats"
	"github.com/go-idp/inlets/internal/server/types"
	"github.com/go-zoox/gormx"
	"github.com/go-zoox/zoox"
)

// API registers admin HTTP routes.
type API struct {
	deps    service.RuntimeDeps
	monitor *service.Monitor
	config  *service.ConfigService
	audit   *service.Audit
}

func New(deps service.RuntimeDeps) *API {
	return &API{
		deps:    deps,
		monitor: service.NewMonitor(deps),
		config:  service.NewConfigService(deps.ConfigPath, deps.ReloadManager),
		audit:   service.NewAudit(),
	}
}

func (a *API) Mount(g *zoox.RouterGroup) {
	g.Get("/status", a.Status)
	g.Get("/overview", a.Overview)
	g.Get("/sessions", a.Sessions)
	g.Get("/domains", a.Domains)
	g.Get("/stats", a.Stats)
	g.Get("/stats/history", a.StatsHistory)
	g.Get("/config", a.GetConfig)
	g.Get("/config/raw", a.GetConfigRaw)
	g.Put("/config", a.PutConfig)
	g.Post("/reload", a.Reload)
	g.Get("/audit", a.AuditList)
}

func (a *API) Status(ctx *zoox.Context) {
	ok(ctx, zoox.H{
		"version":      a.deps.ServerVersion,
		"configPath":   a.deps.ConfigPath,
		"reloadReady":  a.deps.ReloadManager != nil,
		"domain":       a.deps.Domain,
		"httpPort":     a.deps.HTTPPort,
		"tcpPort":      a.deps.TCPPort,
		"sessionCount": len(a.monitor.Sessions()),
	})
}

func (a *API) Overview(ctx *zoox.Context) {
	ok(ctx, a.monitor.Overview())
}

func (a *API) Sessions(ctx *zoox.Context) {
	ok(ctx, a.monitor.Sessions())
}

func (a *API) Domains(ctx *zoox.Context) {
	ok(ctx, a.monitor.Domains())
}

func (a *API) Stats(ctx *zoox.Context) {
	ok(ctx, zoox.H{
		"global":   a.monitor.StatsGlobal(),
		"byClient": a.monitor.StatsByClient(),
	})
}

func (a *API) StatsHistory(ctx *zoox.Context) {
	limit := 48
	if v := ctx.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(string(v)); err == nil && n > 0 {
			limit = n
		}
	}
	var rows []*model.MetricSnapshot
	err := gormx.GetDB().Order("created_at DESC").Limit(limit).Find(&rows).Error
	if err != nil {
		fail(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ok(ctx, rows)
}

func (a *API) GetConfig(ctx *zoox.Context) {
	mask := ctx.Query().Get("maskSecrets") != "false"
	cfg, err := a.config.Document(mask)
	if err != nil {
		fail(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ok(ctx, zoox.H{"path": a.config.Path(), "config": cfg})
}

func (a *API) GetConfigRaw(ctx *zoox.Context) {
	raw, err := a.config.Raw()
	if err != nil {
		fail(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ok(ctx, zoox.H{"path": a.config.Path(), "yaml": string(raw)})
}

type putConfigBody struct {
	YAML string `json:"yaml"`
}

func (a *API) PutConfig(ctx *zoox.Context) {
	var body putConfigBody
	if err := ctx.BindJSON(&body); err != nil {
		raw, err2 := io.ReadAll(ctx.Request.Body)
		if err2 != nil || len(raw) == 0 {
			fail(ctx, http.StatusBadRequest, "invalid JSON body; expected {\"yaml\":\"...\"}")
			return
		}
		body.YAML = string(raw)
	}
	if body.YAML == "" {
		fail(ctx, http.StatusBadRequest, "yaml is required")
		return
	}
	if err := a.config.SaveRaw([]byte(body.YAML)); err != nil {
		fail(ctx, http.StatusBadRequest, err.Error())
		return
	}
	_, _ = a.audit.Record("config.save", "config file updated", actorFromRequest(ctx.Request), ctx.ClientIP())
	ok(ctx, zoox.H{"reloaded": true})
}

func (a *API) Reload(ctx *zoox.Context) {
	if a.deps.ReloadManager == nil {
		fail(ctx, http.StatusServiceUnavailable, "reload manager not configured")
		return
	}
	if err := a.deps.ReloadManager.Reload(); err != nil {
		fail(ctx, http.StatusBadRequest, err.Error())
		return
	}
	_, _ = a.audit.Record("config.reload", "configuration reloaded", actorFromRequest(ctx.Request), ctx.ClientIP())
	ok(ctx, zoox.H{"reloaded": true})
}

func (a *API) AuditList(ctx *zoox.Context) {
	limit := 50
	if v := ctx.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(string(v)); err == nil {
			limit = n
		}
	}
	rows, err := a.audit.List(limit)
	if err != nil {
		fail(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ok(ctx, rows)
}

// RecordSnapshot persists current global traffic stats.
func RecordSnapshot(ctx *types.Context) error {
	if ctx == nil || ctx.TrafficStats == nil || ctx.Container == nil {
		return nil
	}
	raw := ctx.TrafficStats.GetStats("")
	data, ok := raw.(*stats.TrafficStatsData)
	if !ok || data == nil || data.Global == nil {
		return nil
	}
	g := data.Global
	row := &model.MetricSnapshot{
		UploadBytes:   g.UploadBytes,
		DownloadBytes: g.DownloadBytes,
		Requests:      g.Requests,
		Connections:   g.Connections,
		SessionCount:  len(ctx.Container.ListAll()),
	}
	_, err := gormx.Create(row)
	return err
}
