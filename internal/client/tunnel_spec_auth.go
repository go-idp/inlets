package client

import (
	"fmt"
	"strings"
)

// AuthSnapshotFromOptions builds a minimal Authentication for matching this process against server tunnel rows.
func AuthSnapshotFromOptions(o *Options) *Authentication {
	if o == nil {
		return nil
	}
	return &Authentication{
		Type:       o.Type,
		Port:       o.UpstreamPort,
		SubDomain:  o.SubDomain,
		TunnelPort: o.Port,
	}
}

// ApplyTunnelSpecToAuthentication overwrites auth tunnel fields from a server YAML spec (monitor handshake).
func ApplyTunnelSpecToAuthentication(auth *Authentication, spec *TunnelSpec) error {
	if auth == nil || spec == nil {
		return fmt.Errorf("nil auth or spec")
	}
	t := strings.ToLower(strings.TrimSpace(spec.Type))
	if t != "http" && t != "tcp" {
		return fmt.Errorf("invalid tunnel type %q", spec.Type)
	}
	_, port, err := ParseUpstream(spec.Upstream)
	if err != nil {
		return err
	}
	auth.Type = t
	auth.Port = port
	switch t {
	case "http":
		if strings.TrimSpace(spec.SubDomain) != "" {
			auth.SubDomain = strings.TrimSpace(spec.SubDomain)
		}
		auth.TunnelPort = 0
	case "tcp":
		auth.SubDomain = ""
		switch {
		case spec.RemotePort >= 1 && spec.RemotePort <= 65535:
			auth.TunnelPort = spec.RemotePort
		case spec.RemotePort == 0:
			if auth.TunnelPort < 1 || auth.TunnelPort > 65535 {
				return fmt.Errorf("tcp tunnel %q: set remotePort in server config or pass client tunnel port (-p)", spec.Name)
			}
		default:
			return fmt.Errorf("invalid remotePort for tcp tunnel %q", spec.Name)
		}
	}
	return nil
}

// MatchTunnelSpecIndex returns the index of the tunnel spec that matches the incoming auth, or -1.
func MatchTunnelSpecIndex(auth *Authentication, specs []TunnelSpec) int {
	if auth == nil || len(specs) == 0 {
		return -1
	}
	for i := range specs {
		if tunnelSpecMatchesAuth(auth, &specs[i]) {
			return i
		}
	}
	return -1
}

func tunnelSpecMatchesAuth(auth *Authentication, spec *TunnelSpec) bool {
	if spec == nil {
		return false
	}
	t := strings.ToLower(strings.TrimSpace(spec.Type))
	if t != strings.ToLower(strings.TrimSpace(auth.Type)) {
		return false
	}
	if t != "http" && t != "tcp" {
		return false
	}
	_, specPort, err := ParseUpstream(spec.Upstream)
	if err != nil || specPort != auth.Port {
		return false
	}
	switch t {
	case "http":
		if strings.TrimSpace(spec.SubDomain) != "" {
			return strings.TrimSpace(spec.SubDomain) == strings.TrimSpace(auth.SubDomain)
		}
		return true
	case "tcp":
		if spec.RemotePort >= 1 && spec.RemotePort <= 65535 {
			return spec.RemotePort == auth.TunnelPort
		}
		return true
	default:
		return false
	}
}

// SyncOptsFromTunnelSpec updates client Options from a server tunnel spec (bootstrap session).
func SyncOptsFromTunnelSpec(o *Options, spec *TunnelSpec) error {
	if o == nil || spec == nil {
		return fmt.Errorf("nil options or spec")
	}
	host, port, err := ParseUpstream(spec.Upstream)
	if err != nil {
		return err
	}
	t := strings.ToLower(strings.TrimSpace(spec.Type))
	if t != "http" && t != "tcp" {
		return fmt.Errorf("invalid tunnel type %q", spec.Type)
	}
	o.Type = t
	o.UpstreamHost = host
	o.UpstreamPort = port
	switch t {
	case "http":
		if strings.TrimSpace(spec.SubDomain) != "" {
			o.SubDomain = strings.TrimSpace(spec.SubDomain)
		}
		o.Port = 0
	case "tcp":
		o.SubDomain = ""
		switch {
		case spec.RemotePort >= 1 && spec.RemotePort <= 65535:
			o.Port = spec.RemotePort
		case spec.RemotePort == 0:
			if o.Port < 1 || o.Port > 65535 {
				return fmt.Errorf("tcp tunnel %q: set remotePort in server config or pass -p on the client", spec.Name)
			}
		default:
			return fmt.Errorf("invalid remotePort for tcp tunnel %q", spec.Name)
		}
	}
	return nil
}
