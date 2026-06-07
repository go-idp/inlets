package config

import (
	"strings"
	"testing"

	"github.com/go-idp/inlets/internal/client"
)

func TestValidate_NilConfig(t *testing.T) {
	err := Validate(nil)
	if err == nil {
		t.Fatal("expected error for nil config")
	}
	if !strings.Contains(err.Error(), "config is nil") {
		t.Errorf("expected 'config is nil' in error, got: %v", err)
	}
}

func TestValidateWithDetails_DomainRequired(t *testing.T) {
	cfg := &FileConfig{
		Clients: []ClientConfig{{ClientID: "a", ClientSecret: "b"}},
	}
	details := ValidateWithDetails(cfg)
	if len(details) == 0 {
		t.Fatal("expected at least one error")
	}
	found := false
	for _, d := range details {
		if d.Path == "domain" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'domain' path in errors, got: %+v", details)
	}
}

func TestValidateWithDetails_ClientsEmpty(t *testing.T) {
	cfg := &FileConfig{Domain: "example.com"}
	details := ValidateWithDetails(cfg)
	if len(details) == 0 {
		t.Fatal("expected at least one error")
	}
	found := false
	for _, d := range details {
		if d.Path == "clients" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'clients' path, got: %+v", details)
	}
}

func TestValidateWithDetails_ClientFields(t *testing.T) {
	cfg := &FileConfig{
		Domain: "example.com",
		Clients: []ClientConfig{
			{ClientID: "", ClientSecret: "secret"},
			{ClientID: "id2", ClientSecret: ""},
		},
	}
	details := ValidateWithDetails(cfg)
	paths := map[string]bool{}
	for _, d := range details {
		paths[d.Path] = true
	}
	if !paths["clients[0].clientId"] {
		t.Errorf("expected clients[0].clientId error, got paths=%v", paths)
	}
	if !paths["clients[1].clientSecret"] {
		t.Errorf("expected clients[1].clientSecret error, got paths=%v", paths)
	}
}

func TestValidateWithDetails_DuplicateClientID(t *testing.T) {
	cfg := &FileConfig{
		Domain: "example.com",
		Clients: []ClientConfig{
			{ClientID: "dup", ClientSecret: "a"},
			{ClientID: "dup", ClientSecret: "b"},
		},
	}
	details := ValidateWithDetails(cfg)
	found := false
	for _, d := range details {
		if d.Path == "clients[1].clientId" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected duplicate detection, got: %+v", details)
	}
}

func TestValidateWithDetails_TunnelType(t *testing.T) {
	cfg := &FileConfig{
		Domain: "example.com",
		Clients: []ClientConfig{
			{ClientID: "a", ClientSecret: "b", Tunnels: []client.TunnelSpec{
				{Type: "ftp", Upstream: "x"},
			}},
		},
	}
	details := ValidateWithDetails(cfg)
	found := false
	for _, d := range details {
		if strings.HasPrefix(d.Path, "clients[0].tunnels[0].type") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected tunnel.type error, got: %+v", details)
	}
}

func TestValidateWithDetails_ValidConfig(t *testing.T) {
	cfg := &FileConfig{
		Domain: "example.com",
		Clients: []ClientConfig{
			{ClientID: "a", ClientSecret: "b"},
		},
	}
	details := ValidateWithDetails(cfg)
	if len(details) != 0 {
		t.Errorf("expected zero errors for valid config, got: %+v", details)
	}
}

func TestValidateWithDetails_BadDuration(t *testing.T) {
	cfg := &FileConfig{
		Domain: "example.com",
		Clients: []ClientConfig{{ClientID: "a", ClientSecret: "b"}},
		PublicHTTPNoAuth: &PublicHTTPNoAuthConfig{
			Timeout: "not-a-duration",
		},
	}
	details := ValidateWithDetails(cfg)
	found := false
	for _, d := range details {
		if d.Path == "publicHTTPNoAuth.timeout" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected bad duration error, got: %+v", details)
	}
}
