package handler

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/go-idp/inlets/internal/server/admin/model"
	"github.com/go-idp/inlets/internal/server/admin/service"
	"github.com/go-idp/inlets/internal/server/config"
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
	g.Get("/config/schema", a.GetConfigSchema)
	g.Post("/config/validate", a.ValidateConfig)
	g.Put("/config", a.PutConfig)
	g.Post("/reload", a.Reload)
	g.Get("/config/revisions", a.ListRevisions)
	g.Get("/config/revisions/:id", a.GetRevision)
	g.Post("/config/revisions/:id/restore", a.RestoreRevision)
	g.Get("/overrides", a.ListOverrides)
	g.Put("/overrides", a.SetOverride)
	g.Delete("/overrides/clear-all", a.ClearAllOverrides)
	g.Delete("/overrides/*path", a.DeleteOverride)
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

func (a *API) GetConfigSchema(ctx *zoox.Context) {
	ok(ctx, service.NewConfigSchema())
}

func (a *API) ValidateConfig(ctx *zoox.Context) {
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
	cfg, err := parseConfigYAML([]byte(body.YAML))
	if err != nil {
		ok(ctx, zoox.H{"ok": false, "errors": []config.ValidationError{{Path: "", Message: "parse error: " + err.Error()}}})
		return
	}
	details := config.ValidateWithDetails(cfg)
	ok(ctx, zoox.H{"ok": len(details) == 0, "errors": details})
}

type putConfigBody struct {
	YAML    string `json:"yaml"`
	Summary string `json:"summary"`
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
	res, err := a.config.SaveRaw(service.SaveRawInput{
		Raw:      []byte(body.YAML),
		Summary:  body.Summary,
		Actor:    actorFromRequest(ctx.Request),
		ClientIP: ctx.ClientIP(),
	})
	if err != nil {
		fail(ctx, http.StatusBadRequest, err.Error())
		return
	}
	_, _ = a.audit.Record("config.save", "config file updated",
		actorFromRequest(ctx.Request), ctx.ClientIP(), res.Diff)
	ok(ctx, zoox.H{"reloaded": true, "revisionId": res.RevisionID})
}

func (a *API) ListRevisions(ctx *zoox.Context) {
	limit := 20
	if v := ctx.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(string(v)); err == nil && n > 0 {
			limit = n
		}
	}
	rows, err := a.config.Revisions().List(limit)
	if err != nil {
		fail(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ok(ctx, service.ToViews(rows))
}

func (a *API) GetRevision(ctx *zoox.Context) {
	idStr := ctx.Param().Get("id").String()
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		fail(ctx, http.StatusBadRequest, "invalid revision id")
		return
	}
	row, err := a.config.Revisions().Get(uint(id))
	if err != nil {
		fail(ctx, http.StatusNotFound, service.ErrRevisionNotFound.Error())
		return
	}
	ok(ctx, zoox.H{
		"id":        row.ID,
		"createdAt": row.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		"actor":     row.Actor,
		"clientIp":  row.ClientIP,
		"summary":   row.Summary,
		"bytesSize": row.BytesSize,
		"yaml":      row.YAML,
	})
}

type restoreBody struct {
	Summary string `json:"summary"`
}

func (a *API) RestoreRevision(ctx *zoox.Context) {
	idStr := ctx.Param().Get("id").String()
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		fail(ctx, http.StatusBadRequest, "invalid revision id")
		return
	}
	var body restoreBody
	_ = ctx.BindJSON(&body) // body optional
	actor := actorFromRequest(ctx.Request)
	clientIP := ctx.ClientIP()
	// Resolve the from-id for audit metadata.
	fromID := uint(id)
	res, err := a.config.Restore(fromID, service.SaveRawInput{
		Summary:  body.Summary,
		Actor:    actor,
		ClientIP: clientIP,
	})
	if err != nil {
		if errors.Is(err, service.ErrRevisionNotFound) {
			fail(ctx, http.StatusNotFound, err.Error())
			return
		}
		fail(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	_, _ = a.audit.Record("config.restore",
		fmt.Sprintf("restored from #%d", fromID),
		actor, clientIP,
		fmt.Sprintf(`{"fromId":%d,"toId":%d}`, fromID, res.RevisionID))
	ok(ctx, zoox.H{"reloaded": true, "revisionId": res.RevisionID})
}

func (a *API) ListOverrides(ctx *zoox.Context) {
	if a.deps.Override == nil {
		fail(ctx, http.StatusServiceUnavailable, "override layer not configured")
		return
	}
	entries := a.deps.Override.List()
	ok(ctx, zoox.H{
		"entries": entries,
		"size":    a.deps.Override.Size(),
	})
}

type setOverrideBody struct {
	Path  string `json:"path"`
	Value any    `json:"value"`
}

func (a *API) SetOverride(ctx *zoox.Context) {
	if a.deps.Override == nil {
		fail(ctx, http.StatusServiceUnavailable, "override layer not configured")
		return
	}
	var body setOverrideBody
	if err := ctx.BindJSON(&body); err != nil {
		fail(ctx, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.Path == "" {
		fail(ctx, http.StatusBadRequest, "path is required")
		return
	}
	if err := a.deps.Override.Set(body.Path, body.Value); err != nil {
		fail(ctx, http.StatusBadRequest, err.Error())
		return
	}
	_, _ = a.audit.Record("config.override.set",
		fmt.Sprintf("override set: %s", body.Path),
		actorFromRequest(ctx.Request), ctx.ClientIP(), "")
	ok(ctx, zoox.H{"ok": true, "size": a.deps.Override.Size()})
}

func (a *API) DeleteOverride(ctx *zoox.Context) {
	if a.deps.Override == nil {
		fail(ctx, http.StatusServiceUnavailable, "override layer not configured")
		return
	}
	path := ctx.Param().Get("path").String()
	if path == "" {
		fail(ctx, http.StatusBadRequest, "path is required")
		return
	}
	a.deps.Override.Delete(path)
	_, _ = a.audit.Record("config.override.clear",
		fmt.Sprintf("override cleared: %s", path),
		actorFromRequest(ctx.Request), ctx.ClientIP(), "")
	ok(ctx, zoox.H{"ok": true, "size": a.deps.Override.Size()})
}

func (a *API) ClearAllOverrides(ctx *zoox.Context) {
	if a.deps.Override == nil {
		fail(ctx, http.StatusServiceUnavailable, "override layer not configured")
		return
	}
	a.deps.Override.ClearAll()
	_, _ = a.audit.Record("config.override.clear",
		"all overrides cleared",
		actorFromRequest(ctx.Request), ctx.ClientIP(), "")
	ok(ctx, zoox.H{"ok": true, "size": 0})
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
	_, _ = a.audit.Record("config.reload", "configuration reloaded", actorFromRequest(ctx.Request), ctx.ClientIP(), "")
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
