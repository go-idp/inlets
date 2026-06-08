package config

import (
	"time"

	"github.com/go-idp/inlets/internal/client"
	"github.com/go-idp/inlets/internal/server/limiter"
)

// FileConfig is the on-disk inlets server YAML document.
type FileConfig struct {
	Domain           string                     `yaml:"domain" json:"domain"`
	Port             int                        `yaml:"port" json:"port"`
	TCPPort          int                        `yaml:"tcpPort" json:"tcpPort"`
	Secure           *bool                      `yaml:"secure" json:"secure,omitempty"`
	Token            string                     `yaml:"token" json:"token,omitempty"`
	Clients          []ClientConfig             `yaml:"clients" json:"clients,omitempty"`
	Notification     *client.NotificationConfig `yaml:"notification" json:"notification,omitempty"`
	BandwidthLimits  *BandwidthLimitsConfig     `yaml:"bandwidthLimits" json:"bandwidthLimits,omitempty"`
	PublicHTTPNoAuth *PublicHTTPNoAuthConfig    `yaml:"publicHTTPNoAuth,omitempty" json:"publicHTTPNoAuth,omitempty"`
	Admin            *AdminConfig               `yaml:"admin,omitempty" json:"admin,omitempty"`
}

type PublicHTTPNoAuthConfig struct {
	Timeout  string `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	WarnLead string `yaml:"warnLead,omitempty" json:"warnLead,omitempty"`
}

type ClientConfig struct {
	ClientID       string                  `yaml:"clientId" json:"clientId"`
	ClientSecret   string                  `yaml:"clientSecret" json:"clientSecret"`
	Config         *client.Config          `yaml:"config" json:"config,omitempty"`
	BandwidthLimit *limiter.BandwidthLimit `yaml:"bandwidthLimit" json:"bandwidthLimit,omitempty"`
	Tunnels        []client.TunnelSpec     `yaml:"tunnels,omitempty" json:"tunnels,omitempty"`
}

type BandwidthLimitsConfig struct {
	Global  *limiter.BandwidthLimit            `yaml:"global" json:"global,omitempty"`
	Clients map[string]*limiter.BandwidthLimit `yaml:"clients" json:"clients,omitempty"`
}

// AdminConfig controls the optional admin console HTTP server.
type AdminConfig struct {
	Enabled  bool              `yaml:"enabled" json:"enabled"`
	Listen   string            `yaml:"listen" json:"listen,omitempty"`
	Database AdminDatabase     `yaml:"database" json:"database,omitempty"`
	Runtime  AdminRuntime      `yaml:"runtime" json:"runtime,omitempty"`
	UI       AdminUI           `yaml:"ui" json:"ui,omitempty"`
}

type AdminDatabase struct {
	Path string `yaml:"path" json:"path,omitempty"`
}

type AdminRuntime struct {
	PidFile          string `yaml:"pidFile" json:"pidFile,omitempty"`
	SnapshotInterval string `yaml:"snapshotInterval" json:"snapshotInterval,omitempty"`
}

type AdminUI struct {
	BasePath string `yaml:"basePath" json:"basePath,omitempty"`
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

// SameListen reports whether two resolved admin configs bind the same HTTP listener.
func (r *ResolvedAdmin) SameListen(other *ResolvedAdmin) bool {
	if r == nil || other == nil {
		return r == other
	}
	return r.Host == other.Host && r.Port == other.Port && r.UIBasePath == other.UIBasePath
}
