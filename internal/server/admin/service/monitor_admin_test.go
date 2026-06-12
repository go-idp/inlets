package service

import (
	"testing"
	"time"

	"github.com/go-idp/inlets/internal/server/config"
)

func TestAdminStatusView(t *testing.T) {
	disabled := adminStatusView(nil)
	if disabled["enabled"] != false {
		t.Fatalf("expected disabled, got %+v", disabled)
	}

	view := adminStatusView(&config.ResolvedAdmin{
		Enabled:          true,
		Host:             "0.0.0.0",
		Port:             9090,
		DatabasePath:     "/var/lib/inlets/admin.db",
		PidFile:          "/etc/inlets/server.yaml.pid",
		SnapshotInterval: time.Minute,
		UIBasePath:       "/",
	})
	if view["enabled"] != true {
		t.Fatalf("expected enabled, got %+v", view)
	}
	if view["listen"] != ":9090" {
		t.Fatalf("listen = %v, want :9090", view["listen"])
	}
	if view["databasePath"] != "/var/lib/inlets/admin.db" {
		t.Fatalf("databasePath = %v", view["databasePath"])
	}
}
