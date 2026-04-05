package protocol

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"io"
	"strings"

	"github.com/andybalholm/brotli"
	"github.com/go-idp/inlets/internal/client"
)

// compressGzip compresses a string using gzip and returns base64-encoded result
func compressGzip(data string) (string, error) {
	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)

	_, err := writer.Write([]byte(data))
	if err != nil {
		writer.Close()
		return "", err
	}

	if err := writer.Close(); err != nil {
		return "", err
	}

	// Return as base64-encoded string
	return base64Encode(buf.Bytes()), nil
}

// decompressGzip decompresses a base64-encoded gzip string
func decompressGzip(data string) (string, error) {
	// Decode base64
	compressed, err := base64Decode(data)
	if err != nil {
		return "", err
	}

	// Decompress gzip
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return "", err
	}
	defer reader.Close()

	var buf bytes.Buffer
	_, err = io.Copy(&buf, reader)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

// base64Encode encodes bytes to base64 string
func base64Encode(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

// base64Decode decodes base64 string to bytes
func base64Decode(data string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(data)
}

// compressBrotli compresses data using Brotli
func compressBrotli(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	writer := brotli.NewWriter(&buf)

	_, err := writer.Write(data)
	if err != nil {
		writer.Close()
		return nil, err
	}

	if err := writer.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// decompressBrotli decompresses Brotli data
func decompressBrotli(data []byte) ([]byte, error) {
	reader := brotli.NewReader(bytes.NewReader(data))

	var buf bytes.Buffer
	_, err := io.Copy(&buf, reader)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// compressGzipBytes compresses bytes using gzip
func compressGzipBytes(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)

	_, err := writer.Write(data)
	if err != nil {
		writer.Close()
		return nil, err
	}

	if err := writer.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// DecompressBinaryPayloadForCapabilities decompresses a binary frame payload when compression is negotiated (e.g. semantic HTTP head).
func DecompressBinaryPayloadForCapabilities(caps *client.Capabilities, data []byte) ([]byte, error) {
	if caps == nil || caps.Flags&client.CapabilityFlagCompression == 0 || len(data) == 0 {
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
		return decompressBrotli(data)
	}
}

// decompressGzipBytes decompresses gzip bytes
func decompressGzipBytes(data []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	var buf bytes.Buffer
	_, err = io.Copy(&buf, reader)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
