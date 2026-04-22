package client

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBuildConnectionTargetsWithServer(t *testing.T) {
	t.Parallel()

	opts := &Options{
		Server: "https://example.com/base",
	}

	targets, err := buildConnectionTargets(opts)
	if err != nil {
		t.Fatalf("buildConnectionTargets returned error: %v", err)
	}

	if targets.monitorURL != "wss://example.com:443/base/_/monitor" {
		t.Fatalf("unexpected monitorURL: %s", targets.monitorURL)
	}
	if targets.legacyURL != "wss://example.com:443/base/_client" {
		t.Fatalf("unexpected legacyURL: %s", targets.legacyURL)
	}
	if targets.dataBaseURL != "wss://example.com:443/base/_/data" {
		t.Fatalf("unexpected dataBaseURL: %s", targets.dataBaseURL)
	}
	if targets.remoteHost != "example.com" {
		t.Fatalf("unexpected remoteHost: %s", targets.remoteHost)
	}
	if targets.allowLegacyFallback {
		t.Fatalf("server mode must not allow legacy fallback")
	}
}

func TestBuildConnectionTargetsWithRemote(t *testing.T) {
	t.Parallel()

	opts := &Options{
		Remote: "example.com:443",
	}

	targets, err := buildConnectionTargets(opts)
	if err != nil {
		t.Fatalf("buildConnectionTargets returned error: %v", err)
	}

	if targets.monitorURL != "wss://example.com:443/_/monitor" {
		t.Fatalf("unexpected monitorURL: %s", targets.monitorURL)
	}
	if targets.legacyURL != "wss://example.com:443/_client" {
		t.Fatalf("unexpected legacyURL: %s", targets.legacyURL)
	}
	if targets.dataBaseURL != "wss://example.com:443/_/data" {
		t.Fatalf("unexpected dataBaseURL: %s", targets.dataBaseURL)
	}
	if targets.remoteHost != "example.com" {
		t.Fatalf("unexpected remoteHost: %s", targets.remoteHost)
	}
	if !targets.allowLegacyFallback {
		t.Fatalf("legacy remote mode must allow legacy fallback")
	}
}

func TestBuildConnectionTargetsWithInvalidServer(t *testing.T) {
	t.Parallel()

	opts := &Options{
		Server: "https://example.com/base?token=1",
	}

	if _, err := buildConnectionTargets(opts); err == nil {
		t.Fatalf("expected error for server URL with query")
	}
}

func TestBuildConnectionTargetsWithHTTPServerPath(t *testing.T) {
	t.Parallel()

	opts := &Options{
		Server: "http://example.com:8080/base/path",
	}

	targets, err := buildConnectionTargets(opts)
	if err != nil {
		t.Fatalf("buildConnectionTargets returned error: %v", err)
	}

	if targets.monitorURL != "ws://example.com:8080/base/path/_/monitor" {
		t.Fatalf("unexpected monitorURL: %s", targets.monitorURL)
	}
	if targets.legacyURL != "ws://example.com:8080/base/path/_client" {
		t.Fatalf("unexpected legacyURL: %s", targets.legacyURL)
	}
	if targets.dataBaseURL != "ws://example.com:8080/base/path/_/data" {
		t.Fatalf("unexpected dataBaseURL: %s", targets.dataBaseURL)
	}
}

func TestEstablishMonitorWithServer404ReturnsV2Hint(t *testing.T) {
	t.Parallel()

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	}))
	defer s.Close()

	opts := &Options{
		Server: s.URL + "/base",
	}
	targets, err := buildConnectionTargets(opts)
	if err != nil {
		t.Fatalf("buildConnectionTargets returned error: %v", err)
	}

	c := New(opts)
	_, err = c.establishMonitor(targets, true)
	if err == nil {
		t.Fatalf("expected establishMonitor to fail on 404 without legacy fallback")
	}
	if !strings.Contains(err.Error(), "the specified server does not support v2; use --remote and --remote-tcp-port with --legacy") {
		t.Fatalf("unexpected error: %v", err)
	}
}
