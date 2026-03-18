package tunnel

import (
	"bufio"
	"net/http"
	"strings"
	"testing"
)

func TestBuildGatewayTimeoutResponse(t *testing.T) {
	raw := buildGatewayTimeoutResponse()
	reader := bufio.NewReader(strings.NewReader(raw))

	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		t.Fatalf("failed to parse timeout response: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 504 {
		t.Fatalf("expected status 504, got %d", resp.StatusCode)
	}

	if !resp.Close {
		t.Fatal("expected response to close the connection")
	}
}

