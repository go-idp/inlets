package client

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	"github.com/andybalholm/brotli"

	"github.com/go-idp/inlets/internal/legacytunnel"
)

const (
	binaryMessageTypeHTTPRequest      uint8 = 0x01
	binaryMessageTypeHTTPRequestHead  uint8 = 0x07
	binaryMessageTypeHTTPRequestBody  uint8 = 0x08
	binaryMessageTypeHTTPResponseHead uint8 = 0x09
	binaryMessageTypeHTTPResponseBody uint8 = 0x0a
)

// decodeLegacyHTTPRequestPayload reverses server legacy tunnel encoding (Brotli outer, gzip fallback,
// plain base64 for no-op compress). Shared with internal/legacytunnel.
func decodeLegacyHTTPRequestPayload(data string) ([]byte, error) {
	raw, err := legacytunnel.DecodePayload(data)
	if err != nil {
		return nil, fmt.Errorf("legacy tunnel request: %w", err)
	}
	return raw, nil
}

// decodeNewProtocolHTTPRequestPayload decodes a monitor JSON "request" payload from
// BinaryProtocolAdapter.sendDirect (base64-wrapped binary frame, optional brotli/gzip body).
func decodeNewProtocolHTTPRequestPayload(data string, caps *Capabilities) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, fmt.Errorf("new protocol tunnel request: outer base64: %w", err)
	}
	msg, err := parseBinaryMessage(raw)
	if err != nil {
		return nil, fmt.Errorf("new protocol tunnel request: parse binary: %w", err)
	}
	if msg.Type != binaryMessageTypeHTTPRequest {
		return nil, fmt.Errorf("new protocol tunnel request: expected message type %d, got %d", binaryMessageTypeHTTPRequest, msg.Type)
	}
	payload := msg.Data
	if caps != nil && caps.Flags&CapabilityFlagCompression != 0 && len(payload) > 0 {
		alg := "brotli"
		if caps.Features != nil && caps.Features.Compression != nil && caps.Features.Compression.Preferred != "" {
			alg = strings.ToLower(caps.Features.Compression.Preferred)
		}
		var derr error
		switch alg {
		case "gzip":
			payload, derr = decompressGzipBytes(payload)
		case "brotli", "":
			payload, derr = decompressBrotliBytes(payload)
		default:
			payload, derr = decompressBrotliBytes(payload)
		}
		if derr != nil {
			return nil, fmt.Errorf("new protocol tunnel request: decompress (%s): %w", alg, derr)
		}
	}
	return payload, nil
}

func decompressGzipBytes(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decompressBrotliBytes(data []byte) ([]byte, error) {
	r := brotli.NewReader(bytes.NewReader(data))
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// decompressTunnelSemanticHead decompresses a semantic HTTP head frame (server -> client).
func decompressTunnelSemanticHead(data []byte, caps *Capabilities) ([]byte, error) {
	if caps == nil || caps.Flags&CapabilityFlagCompression == 0 || len(data) == 0 {
		return data, nil
	}
	alg := "brotli"
	if caps.Features != nil && caps.Features.Compression != nil && caps.Features.Compression.Preferred != "" {
		alg = strings.ToLower(caps.Features.Compression.Preferred)
	}
	switch alg {
	case "gzip":
		return decompressGzipBytes(data)
	default:
		return decompressBrotliBytes(data)
	}
}
