package utils

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/go-zoox/inlets/internal/server/types"
)

// HMACSHA512 computes HMAC-SHA512 signature
func HMACSHA512(message, secret string) string {
	mac := hmac.New(sha512.New, []byte(secret))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

// CompareVersion compares two version strings
// Returns: -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2
func CompareVersion(v1, v2 string) int {
	// Simple version comparison (can be improved with semver library)
	// For now, just do string comparison
	if v1 < v2 {
		return -1
	} else if v1 > v2 {
		return 1
	}
	return 0
}

// IsVersionGreaterOrEqual checks if v1 >= v2
func IsVersionGreaterOrEqual(v1, v2 string) bool {
	return CompareVersion(v1, v2) >= 0
}

// GetMachineIP gets the machine's public IP address
func GetMachineIP() (string, error) {
	// Try to get IP from a public service
	client := &http.Client{
		Timeout: 3 * time.Second,
	}

	resp, err := client.Get("https://httpbin.zcorky.com/ip/plain")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	buf := make([]byte, 100)
	n, err := resp.Body.Read(buf)
	if err != nil && n == 0 {
		return "", err
	}

	return string(buf[:n]), nil
}

// GetAvailablePort finds an available port
func GetAvailablePort() (int, error) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()

	addr := listener.Addr().(*net.TCPAddr)
	return addr.Port, nil
}

// SetupStatsAPI sets up statistics API endpoints
func SetupStatsAPI(ctx *types.Context) {
	http.HandleFunc("/api/stats", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Get all stats
		statsData := ctx.TrafficStats.GetStats("")
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(statsData)
	})

	http.HandleFunc("/api/stats/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Extract clientId from path
		path := r.URL.Path
		clientId := path[len("/api/stats/"):]

		// Get client stats
		clientStats := ctx.TrafficStats.GetStats(clientId)
		if clientStats == nil {
			http.Error(w, "Client not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(clientStats)
	})
}
