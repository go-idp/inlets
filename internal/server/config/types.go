package config

import (
	"time"

	"github.com/go-idp/inlets/internal/client"
	"github.com/go-idp/inlets/internal/server/limiter"
)

// FileConfig is the on-disk inlets server YAML document.
type FileConfig struct {
	Domain           string                     `yaml:"domain"`
	Port             int                        `yaml:"port"`
	TCPPort          int                        `yaml:"tcpPort"`
	Secure           *bool                      `yaml:"secure"`
	Token            string                     `yaml:"token"`
	Clients          []ClientConfig             `yaml:"clients"`
	Notification     *client.NotificationConfig `yaml:"notification"`
	BandwidthLimits  *BandwidthLimitsConfig     `yaml:"bandwidthLimits"`
	PublicHTTPNoAuth *PublicHTTPNoAuthConfig    `yaml:"publicHTTPNoAuth,omitempty"`
	Admin            *AdminConfig               `yaml:"admin,omitempty"`
}

type PublicHTTPNoAuthConfig struct {
	Timeout  string `yaml:"timeout,omitempty"`
	WarnLead string `yaml:"warnLead,omitempty"`
}

type ClientConfig struct {
	ClientID       string                  `yaml:"clientId"`
	ClientSecret   string                  `yaml:"clientSecret"`
	Config         *client.Config          `yaml:"config"`
	BandwidthLimit *limiter.BandwidthLimit `yaml:"bandwidthLimit"`
	Tunnels        []client.TunnelSpec     `yaml:"tunnels,omitempty"`
}

type BandwidthLimitsConfig struct {
	Global  *limiter.BandwidthLimit            `yaml:"global"`
	Clients map[string]*limiter.BandwidthLimit `yaml:"clients"`
}

// AdminConfig controls the optional admin console HTTP server.
type AdminConfig struct {
	Enabled  bool              `yaml:"enabled"`
	Listen   string            `yaml:"listen"`
	Database AdminDatabase     `yaml:"database"`
	Runtime  AdminRuntime      `yaml:"runtime"`
	UI       AdminUI           `yaml:"ui"`
}

type AdminDatabase struct {
	Path string `yaml:"path"`
}

type AdminRuntime struct {
	PidFile          string `yaml:"pidFile"`
	SnapshotInterval string `yaml:"snapshotInterval"`
}

type AdminUI struct {
	BasePath string `yaml:"basePath"`
}

// ResolvedAdmin holds parsed admin settings with defaults applied.
type ResolvedAdmin struct {
	Enabled          bool
	Host             string
	Port             int
	DatabasePath     string
	PidFile          string
	SnapshotInterval time.Duration
	UIBasePath       string
}
