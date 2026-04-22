package legacytunnel

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"testing"

	"github.com/andybalholm/brotli"
)

func TestEncodeDecodeRoundTrip_Brotli(t *testing.T) {
	raw := []byte("GET /x HTTP/1.1\r\nHost: a\r\n\r\n")
	inner := base64.StdEncoding.EncodeToString(raw)
	wire, err := EncodeOuter(inner)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodePayload(wire)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(raw) {
		t.Fatalf("got %q want %q", got, raw)
	}
	s, err := CallbackWireString(wire)
	if err != nil {
		t.Fatal(err)
	}
	if s != inner {
		t.Fatalf("callback wire got %q want %q", s, inner)
	}
}

func TestDecodePayload_gzipOuter(t *testing.T) {
	raw := []byte("HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n")
	inner := base64.StdEncoding.EncodeToString(raw)
	var gz bytes.Buffer
	w := gzip.NewWriter(&gz)
	if _, err := w.Write([]byte(inner)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	wire := base64.StdEncoding.EncodeToString(gz.Bytes())
	got, err := DecodePayload(wire)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(raw) {
		t.Fatalf("got %q want %q", got, raw)
	}
}

func TestDecodePayload_plainBase64(t *testing.T) {
	raw := []byte("HTTP/1.1 204\r\n\r\n")
	wire := base64.StdEncoding.EncodeToString(raw)
	got, err := DecodePayload(wire)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(raw) {
		t.Fatalf("got %q want %q", got, raw)
	}
}

func TestDecodePayload_rejectsGarbage(t *testing.T) {
	_, err := DecodePayload("!!!")
	if err == nil {
		t.Fatal("expected error")
	}
}

// Brotli-compressed payload must not be mistaken for raw HTTP after first base64 decode.
func TestBrotliNotPlainHTTP(t *testing.T) {
	raw := []byte("GET /z HTTP/1.1\r\nHost: x\r\n\r\n")
	inner := base64.StdEncoding.EncodeToString(raw)
	var buf bytes.Buffer
	bw := brotli.NewWriter(&buf)
	if _, err := bw.Write([]byte(inner)); err != nil {
		t.Fatal(err)
	}
	if err := bw.Close(); err != nil {
		t.Fatal(err)
	}
	// After outer base64 decode, bytes are brotli stream — not ASCII HTTP.
	outer := base64.StdEncoding.EncodeToString(buf.Bytes())
	got, err := DecodePayload(outer)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(raw) {
		t.Fatalf("got %q want %q", got, raw)
	}
}
