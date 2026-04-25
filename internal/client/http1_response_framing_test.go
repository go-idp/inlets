package client

import (
	"net/http"
	"strings"
	"testing"
)

func TestInsertContentLength0IfUnframedEmpty(t *testing.T) {
	raw := "HTTP/1.1 401 Unauthorized\r\n" +
		"WWW-Authenticate: Basic realm=\"x\"\r\n" +
		"Connection: keep-alive\r\n" +
		"\r\n"
	out := string(insertContentLength0IfUnframedEmpty([]byte(raw)))
	if !strings.Contains(out, "Content-Length: 0") {
		t.Fatalf("expected Content-Length: 0, got %q", out)
	}
	// already framed
	already := "HTTP/1.1 404 Not Found\r\nContent-Length: 0\r\nConnection: keep-alive\r\n\r\n"
	out2 := string(insertContentLength0IfUnframedEmpty([]byte(already)))
	if out2 != already {
		t.Fatalf("expected unchanged when CL present, got %q", out2)
	}
	// 204: no change
	s204 := "HTTP/1.1 204 No Content\r\nConnection: keep-alive\r\n\r\n"
	out3 := string(insertContentLength0IfUnframedEmpty([]byte(s204)))
	if out3 != s204 {
		t.Fatalf("expected 204 without insertion, got %q", out3)
	}
}

func TestStatusCodeFromResponseHead(t *testing.T) {
	if c := statusCodeFromResponseHead([]byte("HTTP/1.1 401 Unauthorized\r\nX: y\r\n\r\n")); c != 401 {
		t.Fatalf("got %d", c)
	}
}

func TestResponseHeadForBufferedUpstreamBodyStripsChunked(t *testing.T) {
	// Upstream 401 with chunked + empty (typical) — after ReadAll, body is empty, TE must not remain.
	r := &http.Response{
		Status:        "401 Unauthorized",
		StatusCode:    401,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		ContentLength: -1,
		Header:        http.Header{"Www-Authenticate": []string{`Basic realm="go-zoox"`}},
		TransferEncoding: []string{"chunked"},
	}
	b, err := responseHeadForBufferedUpstreamBody(r, nil)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if strings.Contains(s, "Transfer-Encoding") {
		t.Fatalf("Transfer-Encoding should be stripped, got %q", s)
	}
	if !strings.Contains(s, "Content-Length: 0") {
		t.Fatalf("expected Content-Length: 0, got %q", s)
	}
	if !strings.Contains(s, "Connection: close") {
		t.Fatalf("expected Connection: close for empty message, got %q", s)
	}
}
