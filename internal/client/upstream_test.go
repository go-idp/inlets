package client

import (
	"strings"
	"testing"
)

func TestInjectUpstreamBasicAuth(t *testing.T) {
	raw := []byte("GET / HTTP/1.1\r\nHost: 127.0.0.1:9000\r\n\r\n")
	out := injectUpstreamBasicAuth(raw, "user", "pass")
	outStr := string(out)
	if !strings.Contains(outStr, "Authorization: Basic dXNlcjpwYXNz") {
		t.Fatalf("expected Basic auth for user:pass, got %q", outStr)
	}

	same := injectUpstreamBasicAuth(raw, "", "x")
	if string(same) != string(raw) {
		t.Fatalf("empty username must leave request unchanged")
	}
}
