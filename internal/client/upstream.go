package client

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// injectUpstreamBasicAuth sets Authorization on a raw HTTP/1.x request before it is sent to the local upstream.
func injectUpstreamBasicAuth(raw []byte, username, password string) []byte {
	if strings.TrimSpace(username) == "" {
		return raw
	}
	req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(raw)))
	if err != nil {
		return raw
	}
	defer req.Body.Close()
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(username+":"+password)))
	var buf bytes.Buffer
	if err := req.Write(&buf); err != nil {
		return raw
	}
	return buf.Bytes()
}

// ParseUpstream parses an upstream like the CLI: port only ("9000") or "host:port".
func ParseUpstream(upstream string) (host string, port int, err error) {
	upstream = strings.TrimSpace(upstream)
	if upstream == "" {
		return "", 0, fmt.Errorf("empty upstream")
	}
	allDigits := true
	for _, r := range upstream {
		if r < '0' || r > '9' {
			allDigits = false
			break
		}
	}
	if allDigits {
		port, err = strconv.Atoi(upstream)
		if err != nil || port < 1 || port > 65535 {
			return "", 0, fmt.Errorf("invalid upstream port %q", upstream)
		}
		return "127.0.0.1", port, nil
	}
	parts := strings.Split(upstream, ":")
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("upstream must be port or host:port")
	}
	host = strings.TrimSpace(parts[0])
	if host == "" {
		host = "127.0.0.1"
	}
	port, err = strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || port < 1 || port > 65535 {
		return "", 0, fmt.Errorf("invalid port in upstream %q", upstream)
	}
	return host, port, nil
}
