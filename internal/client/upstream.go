package client

import (
	"fmt"
	"strconv"
	"strings"
)

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
