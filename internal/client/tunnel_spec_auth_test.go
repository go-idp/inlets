package client

import "testing"

func TestMatchTunnelSpecIndex(t *testing.T) {
	specs := []TunnelSpec{
		{Name: "a", Type: "http", Upstream: "127.0.0.1:8080", SubDomain: "web"},
		{Name: "b", Type: "tcp", Upstream: "127.0.0.1:22", RemotePort: 20100},
	}
	auth0 := &Authentication{Type: "http", Port: 8080, SubDomain: "web"}
	if MatchTunnelSpecIndex(auth0, specs) != 0 {
		t.Fatalf("expected index 0")
	}
	auth1 := &Authentication{Type: "tcp", Port: 22, TunnelPort: 20100}
	if MatchTunnelSpecIndex(auth1, specs) != 1 {
		t.Fatalf("expected index 1")
	}
	authMiss := &Authentication{Type: "http", Port: 9999, SubDomain: "x"}
	if MatchTunnelSpecIndex(authMiss, specs) != -1 {
		t.Fatalf("expected no match")
	}

	specsClientTCP := []TunnelSpec{
		{Name: "ssh", Type: "tcp", Upstream: "127.0.0.1:22", RemotePort: 0},
	}
	authTCPAny := &Authentication{Type: "tcp", Port: 22, TunnelPort: 40001}
	if MatchTunnelSpecIndex(authTCPAny, specsClientTCP) != 0 {
		t.Fatalf("tcp remotePort 0 should match client tunnel port")
	}
}
