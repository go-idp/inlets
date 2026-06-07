package handler

import (
	"net/http"
	"strings"
)

func actorFromRequest(r *http.Request) string {
	for _, h := range []string{"X-Forwarded-User", "X-Auth-Request-User", "X-User"} {
		if v := strings.TrimSpace(r.Header.Get(h)); v != "" {
			return v
		}
	}
	return "gateway"
}
