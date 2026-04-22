package client

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"

	"github.com/go-idp/inlets/internal/legacytunnel"
)

// legacyEncodeHTTPRequest mirrors server LegacyProtocolAdapter.encodeRequestData (Brotli outer per zcorky/cliz).
func legacyEncodeHTTPRequestForTest(raw []byte) (string, error) {
	innerB64 := base64.StdEncoding.EncodeToString(raw)
	return legacytunnel.EncodeOuter(innerB64)
}

func TestDecodeLegacyHTTPRequestPayload_roundTrip(t *testing.T) {
	raw := []byte("GET /hello HTTP/1.1\r\nHost: 127.0.0.1\r\n\r\n")
	encoded, err := legacyEncodeHTTPRequestForTest(raw)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeLegacyHTTPRequestPayload(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(got) != string(raw) {
		t.Fatalf("got %q want %q", got, raw)
	}
}

func TestDecodeLegacyHTTPRequestPayload_invalidBase64(t *testing.T) {
	_, err := decodeLegacyHTTPRequestPayload("not-valid-base64!!!")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "outer base64") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func buildNewProtocolWire(t *testing.T, streamID string, msgType uint8, payload []byte) string {
	t.Helper()
	built := buildBinaryMessage(BinaryMessage{
		Type:     msgType,
		StreamID: streamID,
		Sequence: 0,
		Flags:    0,
		Data:     payload,
	})
	return base64.StdEncoding.EncodeToString(built)
}

func TestDecodeNewProtocolHTTPRequestPayload_uncompressed(t *testing.T) {
	raw := []byte("GET / HTTP/1.1\r\nHost: x\r\n\r\n")
	wire := buildNewProtocolWire(t, "tcp1:req1", binaryMessageTypeHTTPRequest, raw)
	caps := &Capabilities{Flags: CapabilityFlagBinaryProtocol}
	got, err := decodeNewProtocolHTTPRequestPayload(wire, caps)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(raw) {
		t.Fatalf("got %q want %q", got, raw)
	}
}

func TestDecodeNewProtocolHTTPRequestPayload_brotliRoundTrip(t *testing.T) {
	raw := []byte("POST /api HTTP/1.1\r\nHost: x\r\nContent-Length: 2\r\n\r\n{}")
	var br bytes.Buffer
	bw := brotli.NewWriter(&br)
	if _, err := bw.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := bw.Close(); err != nil {
		t.Fatal(err)
	}
	wire := buildNewProtocolWire(t, "id", binaryMessageTypeHTTPRequest, br.Bytes())
	caps := &Capabilities{
		Flags: CapabilityFlagBinaryProtocol | CapabilityFlagCompression,
		Features: &CapabilityFeatures{
			Compression: &CompressionFeatures{
				Preferred:  "brotli",
				Algorithms: []string{"brotli", "gzip"},
			},
		},
	}
	got, err := decodeNewProtocolHTTPRequestPayload(wire, caps)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(raw) {
		t.Fatalf("got %q want %q", got, raw)
	}
}

func TestDecodeNewProtocolHTTPRequestPayload_gzipRoundTrip(t *testing.T) {
	raw := []byte("GET /z HTTP/1.1\r\nHost: z\r\n\r\n")
	var gz bytes.Buffer
	w := gzip.NewWriter(&gz)
	if _, err := w.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	wire := buildNewProtocolWire(t, "s", binaryMessageTypeHTTPRequest, gz.Bytes())
	caps := &Capabilities{
		Flags: CapabilityFlagBinaryProtocol | CapabilityFlagCompression,
		Features: &CapabilityFeatures{
			Compression: &CompressionFeatures{
				Preferred:  "gzip",
				Algorithms: []string{"gzip", "brotli"},
			},
		},
	}
	got, err := decodeNewProtocolHTTPRequestPayload(wire, caps)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(raw) {
		t.Fatalf("got %q want %q", got, raw)
	}
}

const binaryMessageTypeTCPData uint8 = 0x03

func TestDecodeNewProtocolHTTPRequestPayload_wrongMessageType(t *testing.T) {
	wire := buildNewProtocolWire(t, "x", binaryMessageTypeTCPData, []byte{1, 2, 3})
	caps := &Capabilities{Flags: CapabilityFlagBinaryProtocol}
	_, err := decodeNewProtocolHTTPRequestPayload(wire, caps)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "expected message type") {
		t.Fatalf("unexpected error: %v", err)
	}
}
