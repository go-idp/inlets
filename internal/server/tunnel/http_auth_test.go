package tunnel

import (
	"encoding/base64"
	"net/http/httptest"
	"testing"

	"github.com/go-idp/inlets/internal/client"
)

func TestIsHTTPRequestAuthorized_NoAuthConfigAllowsRequest(t *testing.T) {
	req := httptest.NewRequest("GET", "http://app.example.com/", nil)
	if !isHTTPRequestAuthorized(req, nil) {
		t.Fatalf("expected request to be authorized when no auths configured")
	}
}

func TestIsHTTPRequestAuthorized_BasicMatches(t *testing.T) {
	req := httptest.NewRequest("GET", "http://app.example.com/", nil)
	raw := base64.StdEncoding.EncodeToString([]byte("username1:password1"))
	req.Header.Set("Authorization", "Basic "+raw)

	auths := []client.HTTPTunnelAuth{
		{Type: "basic", Username: "username1", Password: "password1"},
		{Type: "bearer", Token: "server-token"},
	}
	if !isHTTPRequestAuthorized(req, auths) {
		t.Fatalf("expected basic auth to pass")
	}
}

func TestIsHTTPRequestAuthorized_BearerMatches(t *testing.T) {
	req := httptest.NewRequest("GET", "http://app.example.com/", nil)
	req.Header.Set("Authorization", "Bearer server-token")

	auths := []client.HTTPTunnelAuth{
		{Type: "basic", Username: "username1", Password: "password1"},
		{Type: "bearer", Token: "server-token"},
	}
	if !isHTTPRequestAuthorized(req, auths) {
		t.Fatalf("expected bearer auth to pass")
	}
}

func TestIsHTTPRequestAuthorized_RejectsUnknownAuthorization(t *testing.T) {
	req := httptest.NewRequest("GET", "http://app.example.com/", nil)
	req.Header.Set("Authorization", "Bearer wrong")

	auths := []client.HTTPTunnelAuth{
		{Type: "basic", Username: "username1", Password: "password1"},
		{Type: "bearer", Token: "server-token"},
	}
	if isHTTPRequestAuthorized(req, auths) {
		t.Fatalf("expected request to be rejected")
	}
}
