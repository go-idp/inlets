package protocol

import (
	"encoding/base64"
	"testing"

	"github.com/go-idp/inlets/internal/legacytunnel"
)

func TestLegacyEncodeRequestData_Pre130UsesPlainBase64(t *testing.T) {
	a := NewLegacyProtocolAdapter(nil, false)
	a.SetLegacyPeerVersion("1.2.1")

	raw := []byte("GET / HTTP/1.1\r\nHost: x\r\n\r\n")
	got, err := a.encodeRequestData(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := base64.StdEncoding.EncodeToString(raw)
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestLegacyEncodeRequestData_130AndNewerUseCompressedOuter(t *testing.T) {
	a := NewLegacyProtocolAdapter(nil, false)
	a.SetLegacyPeerVersion("1.3.0")

	raw := []byte("GET / HTTP/1.1\r\nHost: x\r\n\r\n")
	got, err := a.encodeRequestData(raw)
	if err != nil {
		t.Fatal(err)
	}
	plain := base64.StdEncoding.EncodeToString(raw)
	if got == plain {
		t.Fatalf("expected compressed outer payload for 1.3.0+, got plain")
	}
	decoded, err := legacytunnel.DecodePayload(got)
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if string(decoded) != string(raw) {
		t.Fatalf("decoded %q want %q", decoded, raw)
	}
}

func TestLegacySetVersion_SemverFormats(t *testing.T) {
	cases := []struct {
		version string
		want    bool
	}{
		{version: "v1.2.1", want: false},
		{version: "1.2.1-beta.1", want: false},
		{version: "1.3.0", want: true},
		{version: "v1.3.0+build.1", want: true},
	}
	for _, tc := range cases {
		if got := shouldUseCompressedOuter(tc.version); got != tc.want {
			t.Fatalf("version %q got %v want %v", tc.version, got, tc.want)
		}
	}
}
