// Seed admin.demo.db with realistic revision history and audit logs for UI preview.
//
// Usage:
//
//	go run ./scripts/seed-admin-demo -config conf/example/server.demo.yaml
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/go-idp/inlets/internal/server/admin/bootstrap"
	"github.com/go-idp/inlets/internal/server/admin/model"
	"github.com/go-idp/inlets/internal/server/config"
	"github.com/go-zoox/gormx"
)

func main() {
	configPath := flag.String("config", "conf/example/server.demo.yaml", "demo server yaml (for db path resolution)")
	flag.Parse()

	raw, err := config.LoadRaw(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read config: %v\n", err)
		os.Exit(1)
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse config: %v\n", err)
		os.Exit(1)
	}
	if err := config.Validate(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "validate config: %v\n", err)
		os.Exit(1)
	}
	resolved, err := config.ResolveAdmin(cfg, *configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve admin: %v\n", err)
		os.Exit(1)
	}
	dbPath := resolved.DatabasePath
	if err := bootstrap.Init(dbPath); err != nil {
		fmt.Fprintf(os.Stderr, "init db: %v\n", err)
		os.Exit(1)
	}

	db := gormx.GetDB()
	if err := db.Exec("DELETE FROM config_revision").Error; err != nil {
		fmt.Fprintf(os.Stderr, "clear revisions: %v\n", err)
		os.Exit(1)
	}
	if err := db.Exec("DELETE FROM audit_log").Error; err != nil {
		fmt.Fprintf(os.Stderr, "clear audit: %v\n", err)
		os.Exit(1)
	}
	if err := db.Exec("DELETE FROM metric_snapshot").Error; err != nil {
		fmt.Fprintf(os.Stderr, "clear metrics: %v\n", err)
		os.Exit(1)
	}

	current := string(raw)
	revisions := []struct {
		offset  time.Duration
		summary string
		yaml    string
		actor   string
		ip      string
	}{
		{
			offset:  72 * time.Hour,
			summary: "Initial demo setup — prod + staging clients",
			actor:   "ops@acme-corp.io",
			ip:      "10.0.1.12",
			yaml: `domain: tunnel.acme-corp.io
port: 18080
tcpPort: 18443
secure: true
token: demo-shared-token-8k2m9x
clients:
  - clientId: acme-web-prod
    clientSecret: prod-secret-7f3a9c2e1b4d
    tunnels:
      - name: api
        type: http
        upstream: 127.0.0.1:3000
        subDomain: api
  - clientId: acme-web-staging
    clientSecret: staging-secret-b4e81d0a
    tunnels:
      - name: web
        type: http
        upstream: 127.0.0.1:8080
        subDomain: staging
admin:
  enabled: true
  listen: 127.0.0.1:19090
  database:
    path: ./data/admin.demo.db
`,
		},
		{
			offset:  48 * time.Hour,
			summary: "Add bastion TCP tunnels for SSH and Postgres",
			actor:   "platform@acme-corp.io",
			ip:      "10.0.2.8",
			yaml: `domain: tunnel.acme-corp.io
port: 18080
tcpPort: 18443
secure: true
token: demo-shared-token-8k2m9x
clients:
  - clientId: acme-web-prod
    clientSecret: prod-secret-7f3a9c2e1b4d
    tunnels:
      - name: api
        type: http
        upstream: 127.0.0.1:3000
        subDomain: api
      - name: admin
        type: http
        upstream: 127.0.0.1:3001
        subDomain: admin
  - clientId: acme-web-staging
    clientSecret: staging-secret-b4e81d0a
    tunnels:
      - name: web
        type: http
        upstream: 127.0.0.1:8080
        subDomain: staging
  - clientId: infra-bastion
    clientSecret: bastion-secret-x9k2m5p8
    tunnels:
      - name: ssh
        type: tcp
        upstream: 127.0.0.1:22
        remotePort: 22022
admin:
  enabled: true
  listen: 127.0.0.1:19090
  database:
    path: ./data/admin.demo.db
`,
		},
		{
			offset:  24 * time.Hour,
			summary: "Enable global bandwidth limits and Feishu notifications",
			actor:   "ops@acme-corp.io",
			ip:      "10.0.1.12",
			yaml: `domain: tunnel.acme-corp.io
port: 18080
tcpPort: 18443
secure: true
token: demo-shared-token-8k2m9x
clients:
  - clientId: acme-web-prod
    clientSecret: prod-secret-7f3a9c2e1b4d
    bandwidthLimit:
      upload: 5242880
      download: 10485760
    tunnels:
      - name: api
        type: http
        upstream: 127.0.0.1:3000
        subDomain: api
      - name: admin
        type: http
        upstream: 127.0.0.1:3001
        subDomain: admin
  - clientId: acme-web-staging
    clientSecret: staging-secret-b4e81d0a
    tunnels:
      - name: web
        type: http
        upstream: 127.0.0.1:8080
        subDomain: staging
  - clientId: infra-bastion
    clientSecret: bastion-secret-x9k2m5p8
    tunnels:
      - name: ssh
        type: tcp
        upstream: 127.0.0.1:22
        remotePort: 22022
bandwidthLimits:
  global:
    upload: 1048576
    download: 2097152
notification:
  provider: feishu
  url: https://open.feishu.cn/open-apis/bot/v2/hook/demo-webhook-token
admin:
  enabled: true
  listen: 127.0.0.1:19090
  database:
    path: ./data/admin.demo.db
`,
		},
		{
			offset:  6 * time.Hour,
			summary: "Add CI runner and partner webhook clients",
			actor:   "devops@acme-corp.io",
			ip:      "10.0.3.21",
			yaml: current,
		},
	}

	now := time.Now()
	for i, rev := range revisions {
		created := now.Add(-rev.offset)
		row := &model.ConfigRevision{
			CreatedAt: created,
			Actor:     rev.actor,
			ClientIP:  rev.ip,
			Summary:   rev.summary,
			YAML:      rev.yaml,
			BytesSize: len(rev.yaml),
		}
		if err := db.Create(row).Error; err != nil {
			fmt.Fprintf(os.Stderr, "insert revision %d: %v\n", i+1, err)
			os.Exit(1)
		}
	}

	audits := []struct {
		offset  time.Duration
		action  string
		summary string
		actor   string
		ip      string
		diff    string
	}{
		{offset: 71 * time.Hour, action: "config.save", summary: "Initial demo setup — prod + staging clients", actor: "ops@acme-corp.io", ip: "10.0.1.12",
			diff: " domain: tunnel.acme-corp.io\n+clients:\n+  - clientId: acme-web-prod\n+  - clientId: acme-web-staging\n"},
		{offset: 47 * time.Hour, action: "config.save", summary: "Add bastion TCP tunnels for SSH and Postgres", actor: "platform@acme-corp.io", ip: "10.0.2.8",
			diff: " clients:\n   - clientId: acme-web-prod\n+    tunnels:\n+      - name: admin\n+  - clientId: infra-bastion\n+    tunnels:\n+      - name: ssh\n"},
		{offset: 23 * time.Hour, action: "config.save", summary: "Enable global bandwidth limits and Feishu notifications", actor: "ops@acme-corp.io", ip: "10.0.1.12",
			diff: "+bandwidthLimits:\n+  global:\n+    upload: 1048576\n+notification:\n+  provider: feishu\n"},
		{offset: 5 * time.Hour, action: "config.save", summary: "Add CI runner and partner webhook clients", actor: "devops@acme-corp.io", ip: "10.0.3.21",
			diff: "+  - clientId: ci-runner-01\n+  - clientId: partner-webhook\n+publicHTTPNoAuth:\n+  timeout: 15m\n"},
		{offset: 2 * time.Hour, action: "config.override.set", summary: "Temporary: raise prod upload cap for release window", actor: "ops@acme-corp.io", ip: "10.0.1.12", diff: ""},
		{offset: 90 * time.Minute, action: "config.override.clear", summary: "Clear override clients[0].bandwidthLimit.upload", actor: "ops@acme-corp.io", ip: "10.0.1.12", diff: ""},
		{offset: 45 * time.Minute, action: "config.validate", summary: "Manual validation before change window", actor: "platform@acme-corp.io", ip: "10.0.2.8", diff: ""},
	}

	for i, a := range audits {
		row := &model.AuditLog{
			CreatedAt: now.Add(-a.offset),
			Action:    a.action,
			Summary:   a.summary,
			Actor:     a.actor,
			ClientIP:  a.ip,
			Diff:      a.diff,
		}
		if err := db.Create(row).Error; err != nil {
			fmt.Fprintf(os.Stderr, "insert audit %d: %v\n", i+1, err)
			os.Exit(1)
		}
	}

	// Traffic snapshots for the stats page.
	for h := 24; h >= 1; h-- {
		snap := &model.MetricSnapshot{
			CreatedAt:     now.Add(-time.Duration(h) * time.Hour),
			UploadBytes:   int64(1200000 + h*85000),
			DownloadBytes: int64(4800000 + h*120000),
			Requests:      int64(800 + h*45),
			Connections:   int64(12 + h%5),
			SessionCount:  2 + h%3,
		}
		if err := db.Create(snap).Error; err != nil {
			fmt.Fprintf(os.Stderr, "insert metric: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Printf("Seeded demo admin DB: %s\n", dbPath)
	fmt.Printf("  %d config revisions, %d audit logs, 24 metric snapshots\n", len(revisions), len(audits))
}
