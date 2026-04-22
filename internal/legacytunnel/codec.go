// Package legacytunnel implements the JSON monitor-channel tunnel encoding used by
// legacy (pre-capability-negotiation) clients, including zcorky/cliz Node clients:
// wire = base64( compress( utf8(innerBase64) ) ) where compress defaults to Brotli.
// Older Go releases used gzip for the outer layer; we decode with Brotli first, then gzip.
package legacytunnel

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	"github.com/andybalholm/brotli"
)

// EncodeOuter matches zcorky/cliz ProcessServerData.request / ProcessClientData.response:
// inner is the base64 string of the raw HTTP message (ASCII); compress with Brotli; outer base64.
func EncodeOuter(innerBase64 string) (string, error) {
	var buf bytes.Buffer
	w := brotli.NewWriter(&buf)
	if _, err := w.Write([]byte(innerBase64)); err != nil {
		_ = w.Close()
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// DecodePayload reverses EncodeOuter and supports:
//   - Brotli outer (Node / current Go server),
//   - gzip outer (older Go↔Go),
//   - single-layer base64(raw HTTP) when the peer used no compression (Go client no-op compress).
func DecodePayload(wire string) ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(wire)
	if err != nil {
		return nil, fmt.Errorf("legacytunnel: outer base64: %w", err)
	}
	if looksLikeRawHTTP(b) {
		return b, nil
	}

	inner, err := decompressBrotli(b)
	if err != nil {
		inner, err = decompressGzipBytes(b)
		if err != nil {
			return nil, fmt.Errorf("legacytunnel: decompress outer (tried brotli, gzip): %w", err)
		}
	}

	raw, err := base64.StdEncoding.DecodeString(string(inner))
	if err != nil {
		return nil, fmt.Errorf("legacytunnel: inner base64: %w", err)
	}
	return raw, nil
}

// CallbackWireString returns the payload expected by tunnel/http wrappedCallback:
// base64-encoded raw HTTP bytes (net/http-compatible with Node monitor path).
func CallbackWireString(wire string) (string, error) {
	raw, err := DecodePayload(wire)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

func decompressBrotli(compressed []byte) ([]byte, error) {
	r := brotli.NewReader(bytes.NewReader(compressed))
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decompressGzipBytes(compressed []byte) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, zr); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func looksLikeRawHTTP(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	const max = 24
	head := b
	if len(head) > max {
		head = head[:max]
	}
	h := string(head)
	if strings.HasPrefix(h, "HTTP/") {
		return true
	}
	switch {
	case strings.HasPrefix(h, "GET "),
		strings.HasPrefix(h, "POST "),
		strings.HasPrefix(h, "PUT "),
		strings.HasPrefix(h, "DELETE "),
		strings.HasPrefix(h, "HEAD "),
		strings.HasPrefix(h, "OPTIONS "),
		strings.HasPrefix(h, "CONNECT "),
		strings.HasPrefix(h, "PATCH "),
		strings.HasPrefix(h, "TRACE "):
		return true
	default:
		return false
	}
}
