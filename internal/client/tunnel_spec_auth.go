package client

import (
	"fmt"
	"strings"
)

func tunnelPortFromOptions(o *Options) int {
	if o == nil || !strings.EqualFold(o.Type, "tcp") {
		return 0
	}
	return o.TunnelPort
}

func tunnelSpecLabel(spec *TunnelSpec) string {
	if spec == nil {
		return "?"
	}
	if n := strings.TrimSpace(spec.Name); n != "" {
		return n
	}
	if u := strings.TrimSpace(spec.Upstream); u != "" {
		return u
	}
	if t := strings.TrimSpace(spec.Type); t != "" {
		return t
	}
	return "?"
}

// AuthSnapshotFromOptions builds a minimal Authentication for matching this process against server tunnel rows.
func AuthSnapshotFromOptions(o *Options) *Authentication {
	if o == nil {
		return nil
	}
	return &Authentication{
		Type:       o.Type,
		Port:       o.UpstreamPort,
		SubDomain:  o.SubDomain,
		TunnelPort: tunnelPortFromOptions(o),
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
				return fmt.Errorf("tcp tunnel %q: set remotePort in server config or pass tunnel port (tcp -p)", tunnelSpecLabel(spec))
			}
		default:
			return fmt.Errorf("invalid remotePort for tcp tunnel %q", tunnelSpecLabel(spec))
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
		o.TunnelPort = 0
	case "tcp":
		o.SubDomain = ""
		switch {
		case spec.RemotePort >= 1 && spec.RemotePort <= 65535:
			o.TunnelPort = spec.RemotePort
		case spec.RemotePort == 0:
			if o.TunnelPort < 1 || o.TunnelPort > 65535 {
				return fmt.Errorf("tcp tunnel %q: set remotePort in server config or pass tcp -p on the client", tunnelSpecLabel(spec))
			}
		default:
			return fmt.Errorf("invalid remotePort for tcp tunnel %q", tunnelSpecLabel(spec))
		}
	}
	return nil
}
