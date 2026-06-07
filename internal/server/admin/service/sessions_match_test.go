package service

import (
	"testing"

	"github.com/go-idp/inlets/internal/server/config"
	"github.com/go-idp/inlets/internal/server/types"
)

func TestFindClientMatch_Exact(t *testing.T) {
	cfg := &config.FileConfig{
		Domain: "x.com",
		Clients: []config.ClientConfig{
			{ClientID: "alice", ClientSecret: "secret-a"},
		},
	}
	tm := &types.TunnelMapping{
		Type:     types.TunnelTypeHTTP,
		ClientId: "alice",
	}
	_, issues, status := findClientMatch(cfg, tm)
	if status != "exact" {
		t.Errorf("expected exact, got %q (issues=%v)", status, issues)
	}
	if len(issues) != 0 {
		t.Errorf("expected no issues, got %v", issues)
	}
}

func TestFindClientMatch_Missing(t *testing.T) {
	cfg := &config.FileConfig{
		Domain: "x.com",
		Clients: []config.ClientConfig{
			{ClientID: "alice", ClientSecret: "secret-a"},
		},
	}
	tm := &types.TunnelMapping{ClientId: "bob"}
	idx, issues, status := findClientMatch(cfg, tm)
	if status != "missing" {
		t.Errorf("expected missing, got %q", status)
	}
	if idx != -1 {
		t.Errorf("expected idx=-1, got %d", idx)
	}
	if len(issues) == 0 {
		t.Errorf("expected issues")
	}
}

func TestFindClientMatch_Anonymous(t *testing.T) {
	cfg := &config.FileConfig{Domain: "x.com"}
	tm := &types.TunnelMapping{ClientId: "anonymous-abc123"}
	_, issues, status := findClientMatch(cfg, tm)
	if status != "missing" {
		t.Errorf("expected missing, got %q", status)
	}
	if len(issues) == 0 {
		t.Errorf("expected issues for anonymous")
	}
}

func TestFindClientMatch_Partial_EmptySecret(t *testing.T) {
	cfg := &config.FileConfig{
		Domain: "x.com",
		Clients: []config.ClientConfig{
			{ClientID: "alice", ClientSecret: ""},
		},
	}
	tm := &types.TunnelMapping{ClientId: "alice"}
	_, issues, status := findClientMatch(cfg, tm)
	if status != "partial" {
		t.Errorf("expected partial, got %q", status)
	}
	if len(issues) == 0 {
		t.Errorf("expected at least one issue")
	}
}

func TestFindClientMatch_IndexResolution(t *testing.T) {
	cfg := &config.FileConfig{
		Domain: "x.com",
		Clients: []config.ClientConfig{
			{ClientID: "alice", ClientSecret: "s1"},
			{ClientID: "bob", ClientSecret: "s2"},
			{ClientID: "carol", ClientSecret: "s3"},
		},
	}
	tm := &types.TunnelMapping{ClientId: "carol"}
	idx, _, status := findClientMatch(cfg, tm)
	if status != "exact" {
		t.Errorf("expected exact, got %q", status)
	}
	if idx != 2 {
		t.Errorf("expected idx=2, got %d", idx)
	}
}
