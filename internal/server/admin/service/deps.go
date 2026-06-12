package service

import (
	"time"

	"github.com/go-idp/inlets/internal/server/config"
	"github.com/go-idp/inlets/internal/server/types"
)

// RuntimeDeps holds shared admin runtime dependencies.
type RuntimeDeps struct {
	Ctx           *types.Context
	Domain        string
	HTTPPort      int
	TCPPort       int
	Secure        bool
	ServerVersion string
	Started       time.Time
	ConfigPath    string
	ReloadManager *config.Manager
	Admin         *config.ResolvedAdmin
}
