package config

import (
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultAdminListen   = "127.0.0.1:9090"
	defaultAdminDB       = "data/admin.db"
	defaultSnapshotEvery = time.Minute
)

// ResolveAdmin applies defaults for admin settings.
func ResolveAdmin(cfg *FileConfig, configPath string) (*ResolvedAdmin, error) {
	if cfg == nil || cfg.Admin == nil || !cfg.Admin.Enabled {
		return nil, nil
	}
	a := cfg.Admin
	host, port, err := parseListen(a.Listen)
	if err != nil {
		return nil, err
	}
	dbPath := strings.TrimSpace(a.Database.Path)
	if dbPath == "" {
		dbPath = defaultAdminDB
	}
	if !filepath.IsAbs(dbPath) {
		dbPath = filepath.Join(Dir(configPath), dbPath)
	}
	pidFile := strings.TrimSpace(a.Runtime.PidFile)
	if pidFile == "" {
		pidFile = configPath + ".pid"
	} else if !filepath.IsAbs(pidFile) {
		pidFile = filepath.Join(Dir(configPath), pidFile)
	}
	snap := defaultSnapshotEvery
	if v := strings.TrimSpace(a.Runtime.SnapshotInterval); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("invalid admin.runtime.snapshotInterval: %w", err)
		}
		if d > 0 {
			snap = d
		}
	}
	basePath := strings.TrimSpace(a.UI.BasePath)
	if basePath == "" {
		basePath = "/"
	}
	if !strings.HasPrefix(basePath, "/") {
		basePath = "/" + basePath
	}
	return &ResolvedAdmin{
		Enabled:          true,
		Host:             host,
		Port:             port,
		DatabasePath:     dbPath,
		PidFile:          pidFile,
		SnapshotInterval: snap,
		UIBasePath:       basePath,
	}, nil
}

func parseListen(listen string) (host string, port int, err error) {
	v := strings.TrimSpace(listen)
	if v == "" {
		v = defaultAdminListen
	}
	if !strings.Contains(v, ":") {
		v = "127.0.0.1:" + v
	}
	h, p, err := net.SplitHostPort(v)
	if err != nil {
		return "", 0, fmt.Errorf("invalid admin.listen %q: %w", listen, err)
	}
	if h == "" {
		h = "127.0.0.1"
	}
	portNum, err := net.LookupPort("tcp", p)
	if err != nil {
		return "", 0, fmt.Errorf("invalid admin.listen port: %w", err)
	}
	return h, portNum, nil
}
