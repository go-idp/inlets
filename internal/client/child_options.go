package client

import (
	"fmt"
	"strings"
)

// ChildOptionsFromSpec builds options for an additional monitor session from a server tunnel spec.
func ChildOptionsFromSpec(base *Options, spec *TunnelSpec) (*Options, error) {
	if base == nil || spec == nil {
		return nil, fmt.Errorf("nil options or spec")
	}
	t := strings.ToLower(strings.TrimSpace(spec.Type))
	if t != "http" && t != "tcp" {
		return nil, fmt.Errorf("invalid tunnel type %q", spec.Type)
	}
	host, port, err := ParseUpstream(spec.Upstream)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(spec.Name) == "" {
		return nil, fmt.Errorf("tunnel name is required")
	}
	o := &Options{
		Type:                t,
		UpstreamHost:        host,
		UpstreamPort:        port,
		UpstreamUsername:    base.UpstreamUsername,
		UpstreamPassword:    base.UpstreamPassword,
		AuthType:            base.AuthType,
		Token:               base.Token,
		ClientId:            base.ClientId,
		ClientSecret:        base.ClientSecret,
		SubDomain:           strings.TrimSpace(spec.SubDomain),
		Port:                spec.RemotePort,
		Remote:              base.Remote,
		RemoteTCPPort:       base.RemoteTCPPort,
		HealthcheckInt:      base.HealthcheckInt,
		ReportURL:           base.ReportURL,
		Version:             base.Version,
		ReconnectMaxRetries: base.ReconnectMaxRetries,
		ReconnectInterval:   base.ReconnectInterval,
		OpaqueChild:         true,
	}
	if t == "http" {
		o.Port = 0
		return o, nil
	}
	o.SubDomain = ""
	switch {
	case spec.RemotePort >= 1 && spec.RemotePort <= 65535:
		o.Port = spec.RemotePort
	case spec.RemotePort == 0:
		return nil, fmt.Errorf("tcp tunnel %q: set remotePort in server config for automatically started extra tunnels (or run a separate client with -p)", spec.Name)
	default:
		return nil, fmt.Errorf("invalid remotePort for tcp tunnel %q", spec.Name)
	}
	return o, nil
}
