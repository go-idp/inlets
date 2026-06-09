package monitor

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-idp/inlets/internal/client"
	"github.com/go-idp/inlets/internal/server/config"
	servercontainer "github.com/go-idp/inlets/internal/server/container"
	"github.com/go-idp/inlets/internal/server/stats"
	"github.com/go-idp/inlets/internal/server/types"
	"github.com/go-idp/inlets/internal/server/utils"
	"github.com/gorilla/websocket"
)

func TestHandleResponseConsumesCallbackOnce(t *testing.T) {
	ctx := &types.Context{
		CallbackContainer: servercontainer.NewCallbackContainer(),
	}

	tcpID := "tcp-1"
	requestID := "req-1"
	payloadData := "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok"

	callCount := 0
	var got string
	ctx.CallbackContainer.Set(tcpID, requestID, func(data string) {
		callCount++
		got = data
	})

	// New protocol also uses the same TextMessage ["response", ...] path on the monitor channel.
	wsConn := &WebSocketConnection{
		UseNewProtocol: true,
	}
	payload := map[string]interface{}{
		"id":   tcpID + ":" + requestID,
		"data": base64.StdEncoding.EncodeToString([]byte(payloadData)),
	}

	handleResponse(ctx, wsConn, payload)
	handleResponse(ctx, wsConn, payload)

	if callCount != 1 {
		t.Fatalf("expected callback to be called once, got %d", callCount)
	}

	// handleResponse normalizes plain base64(raw HTTP) to the same base64 string (see legacytunnel.CallbackWireString).
	expectedB64 := base64.StdEncoding.EncodeToString([]byte(payloadData))
	if got != expectedB64 {
		t.Fatalf("unexpected callback payload: %q want %q", got, expectedB64)
	}
}

func TestResolveMatchedHTTPAuths_FallbackBySubdomain(t *testing.T) {
	auth := &client.Authentication{
		Type:      "http",
		SubDomain: "myapp",
		Port:      8080, // does not match spec upstream port
	}
	cfg := &client.Config{
		Tunnels: []client.TunnelSpec{
			{
				Name:      "web",
				Type:      "http",
				Upstream:  "127.0.0.1:9000",
				SubDomain: "myapp",
				Auth: &client.HTTPIncomingAuthRule{
					Enable: true,
					Users: []client.HTTPTunnelAuth{
						{Type: "bearer", Token: "server-token"},
					},
				},
			},
		},
	}

	got := resolveMatchedHTTPAuths(auth, cfg)
	if len(got) != 1 || got[0].Type != "bearer" || got[0].Token != "server-token" {
		t.Fatalf("unexpected matched auths: %+v", got)
	}
}

func TestResolveMatchedHTTPAuths_FallbackSingleHTTPAuthSpec(t *testing.T) {
	auth := &client.Authentication{
		Type:      "http",
		SubDomain: "random",
		Port:      7777,
	}
	cfg := &client.Config{
		Tunnels: []client.TunnelSpec{
			{
				Name:     "one-http",
				Type:     "http",
				Upstream: "127.0.0.1:9000",
				Auth: &client.HTTPIncomingAuthRule{
					Enable: true,
					Users: []client.HTTPTunnelAuth{
						{Type: "basic", Username: "u", Password: "p"},
					},
				},
			},
			{
				Name:       "one-tcp",
				Type:       "tcp",
				Upstream:   "127.0.0.1:22",
				RemotePort: 20200,
			},
		},
	}

	got := resolveMatchedHTTPAuths(auth, cfg)
	if len(got) != 1 || got[0].Type != "basic" || got[0].Username != "u" {
		t.Fatalf("unexpected matched auths: %+v", got)
	}
}

func TestRequiresModernClientForAdvancedFeatures(t *testing.T) {
	cfgWithTunnels := &client.Config{
		Tunnels: []client.TunnelSpec{
			{Name: "web", Type: "http", Upstream: "127.0.0.1:9000"},
		},
	}
	if blocked, _ := requiresModernClientForAdvancedFeatures("1.2.1", &client.Authentication{Type: "http"}, cfgWithTunnels); !blocked {
		t.Fatalf("expected old client to be blocked when tunnels are configured")
	}

	cfgWithHTTPAuth := &client.Config{
		Tunnels: []client.TunnelSpec{
			{
				Name:      "web",
				Type:      "http",
				Upstream:  "127.0.0.1:9000",
				SubDomain: "myapp",
				Auth: &client.HTTPIncomingAuthRule{
					Enable: true,
					Users: []client.HTTPTunnelAuth{
						{Type: "bearer", Token: "t"},
					},
				},
			},
		},
	}
	if blocked, _ := requiresModernClientForAdvancedFeatures("1.2.1", &client.Authentication{Type: "http", SubDomain: "myapp"}, cfgWithHTTPAuth); !blocked {
		t.Fatalf("expected old client to be blocked when HTTP auth is configured")
	}

	if blocked, _ := requiresModernClientForAdvancedFeatures("2.0.0", &client.Authentication{Type: "http", SubDomain: "myapp"}, cfgWithHTTPAuth); blocked {
		t.Fatalf("did not expect modern client to be blocked")
	}

	if blocked, _ := requiresModernClientForAdvancedFeatures("1.2.1", &client.Authentication{
		Type: "http",
		HTTPIngressBasic: &client.HTTPTunnelAuth{Type: "basic", Username: "u", Password: "p"},
	}, nil); !blocked {
		t.Fatalf("expected old client to be blocked when client declares HTTP ingress Basic")
	}
	if blocked, _ := requiresModernClientForAdvancedFeatures("2.0.0", &client.Authentication{
		Type: "http",
		HTTPIngressBasic: &client.HTTPTunnelAuth{Type: "basic", Username: "u", Password: "p"},
	}, nil); blocked {
		t.Fatalf("did not expect modern client to be blocked for client ingress auth")
	}
}

func TestMergeHTTPIngressEdgeAuth(t *testing.T) {
	server := []client.HTTPTunnelAuth{{Type: "bearer", Token: "t"}}
	auth := &client.Authentication{
		Type: "http",
		HTTPIngressBasic: &client.HTTPTunnelAuth{
			Type: "basic", Username: "u", Password: "p",
		},
	}
	got := mergeHTTPIngressEdgeAuth(server, auth)
	if len(got) != 1 || got[0].Type != "bearer" {
		t.Fatalf("server auth must win: %+v", got)
	}

	got = mergeHTTPIngressEdgeAuth(nil, auth)
	if len(got) != 1 || got[0].Type != "basic" || got[0].Username != "u" || got[0].Password != "p" {
		t.Fatalf("expected client fallback: %+v", got)
	}

	got = mergeHTTPIngressEdgeAuth(nil, &client.Authentication{Type: "http"})
	if len(got) != 0 {
		t.Fatalf("expected empty without httpIngressBasic: %+v", got)
	}
}

func TestShouldApplyPublicMonitorSessionTTL(t *testing.T) {
	// Unauthenticated (public) monitor: apply regardless of tunnel type or edge auth.
	if !shouldApplyPublicMonitorSessionTTL(&client.Authentication{Type: "http", AuthType: "public"}) {
		t.Fatalf("expected session TTL for public http")
	}
	if !shouldApplyPublicMonitorSessionTTL(&client.Authentication{Type: "http"}) {
		t.Fatalf("expected session TTL for empty authType (treat as public)")
	}
	if !shouldApplyPublicMonitorSessionTTL(&client.Authentication{Type: "tcp", AuthType: "public"}) {
		t.Fatalf("expected session TTL for public tcp (protocol is monitor-only; tunnel type irrelevant)")
	}
	// Token / credentials on monitor: no temp-user session cap.
	if shouldApplyPublicMonitorSessionTTL(&client.Authentication{Type: "http", AuthType: "credentials"}) {
		t.Fatalf("did not expect TTL with credentials")
	}
	if shouldApplyPublicMonitorSessionTTL(&client.Authentication{Type: "http", AuthType: "token"}) {
		t.Fatalf("did not expect TTL with token")
	}
	if shouldApplyPublicMonitorSessionTTL(nil) {
		t.Fatalf("did not expect TTL for nil auth")
	}
}

func TestResolvePublicMonitorSessionTTL(t *testing.T) {
	ttl, warn := resolvePublicMonitorSessionTTL(0, 0)
	if ttl != 10*time.Minute || warn != 2*time.Minute {
		t.Fatalf("expected defaults 10m/2m, got %s/%s", ttl, warn)
	}

	ttl, warn = resolvePublicMonitorSessionTTL(15*time.Minute, 5*time.Minute)
	if ttl != 15*time.Minute || warn != 5*time.Minute {
		t.Fatalf("expected custom values to be preserved, got %s/%s", ttl, warn)
	}

	ttl, warn = resolvePublicMonitorSessionTTL(90*time.Second, 2*time.Minute)
	if warn >= ttl {
		t.Fatalf("warn must be less than ttl, got %s/%s", ttl, warn)
	}
}

func TestSchedulePublicMonitorSessionTTLWarnAndClose(t *testing.T) {
	serverConnCh := make(chan *websocket.Conn, 1)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		serverConnCh <- c
	}))
	defer s.Close()

	wsURL := "ws" + strings.TrimPrefix(s.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	defer clientConn.Close()

	var serverConn *websocket.Conn
	select {
	case serverConn = <-serverConnCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for upgraded server websocket")
	}
	defer serverConn.Close()

	schedulePublicMonitorSessionTTL(&WebSocketConnection{Conn: serverConn}, "client-test", 300*time.Millisecond, 150*time.Millisecond)

	clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var warns []string
	closed := false
	for i := 0; i < 6; i++ {
		mt, msg, err := clientConn.ReadMessage()
		if err != nil {
			closed = true
			break
		}
		if mt != websocket.TextMessage {
			continue
		}
		var arr []interface{}
		if err := json.Unmarshal(msg, &arr); err != nil || len(arr) < 2 {
			continue
		}
		event, _ := arr[0].(string)
		if event != "warn" {
			continue
		}
		warnMsg, _ := arr[1].(string)
		warns = append(warns, warnMsg)
	}

	if len(warns) < 2 {
		t.Fatalf("expected at least 2 warn messages, got %d: %#v", len(warns), warns)
	}
	if !strings.Contains(warns[0], "Unauthenticated (public) monitor session") {
		t.Fatalf("expected first warning to mention unauthenticated public session, got %q", warns[0])
	}
	if !strings.Contains(warns[1], "will close in") {
		t.Fatalf("expected second warning to mention lead close warning, got %q", warns[1])
	}
	if !closed {
		t.Fatalf("expected websocket to be closed after ttl")
	}
}

func TestHandleAuthenticate_CoordinatorSession(t *testing.T) {
	configRef := config.NewRef(&config.FileConfig{
		Token: "server-token",
		Clients: []config.ClientConfig{{
			ClientID:     "agent.idp.ys",
			ClientSecret: "secret1",
			Tunnels: []client.TunnelSpec{{
				Name:     "web",
				Type:     "http",
				Upstream: "127.0.0.1:8080",
			}},
		}},
	})
	getToken := config.CreateGetToken(configRef, "2.0.0")

	ctx := &types.Context{
		Container:         servercontainer.NewTunnelContainer(),
		DomainMappings:    servercontainer.NewDomainContainer(),
		TrafficStats:      stats.NewTrafficStatsContainer(),
		CallbackContainer: servercontainer.NewCallbackContainer(),
	}

	serverConnCh := make(chan *websocket.Conn, 1)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		serverConnCh <- c
	}))
	defer s.Close()

	wsURL := "ws" + strings.TrimPrefix(s.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	defer clientConn.Close()

	var serverConn *websocket.Conn
	select {
	case serverConn = <-serverConnCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for server websocket")
	}
	defer serverConn.Close()

	wsConn := &WebSocketConnection{Conn: serverConn}
	ts := time.Now().UnixMilli()
	authPayload := map[string]interface{}{
		"version":      "2.0.0",
		"type":         "",
		"port":         0,
		"timestamp":    ts,
		"authType":     "credentials",
		"clientId":     "agent.idp.ys",
		"signature":    utils.HMACSHA512(strconv.FormatInt(ts, 10), "secret1"),
		"capabilities": client.GetClientCapabilities("2.0.0"),
	}

	var subDomain string
	var isAuth bool
	err = handleAuthenticate(ctx, &CreateWebSocketOptions{
		Version: "2.0.0",
		Domain:  "example.com",
		Token:   getToken,
	}, NewEventEmitter(), wsConn, authPayload, &isAuth, &subDomain)
	if err != nil {
		t.Fatalf("handleAuthenticate coordinator: %v", err)
	}

	clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := clientConn.ReadMessage()
	if err != nil {
		t.Fatalf("read auth response: %v", err)
	}
	var arr []interface{}
	if err := json.Unmarshal(msg, &arr); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(arr) < 2 || arr[0] != "authenticate" {
		t.Fatalf("unexpected message: %#v", arr)
	}
	resp, ok := arr[1].(map[string]interface{})
	if !ok || resp["ok"] != true {
		t.Fatalf("expected ok authenticate response, got %#v", arr[1])
	}
	cfg, ok := resp["config"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected config in response, got %#v", resp["config"])
	}
	tunnels, ok := cfg["tunnels"].([]interface{})
	if !ok || len(tunnels) != 1 {
		t.Fatalf("expected one tunnel in config, got %#v", cfg["tunnels"])
	}
	if got := ctx.Container.Get(wsConn.ContainerID); got != nil {
		t.Fatalf("coordinator session should not create a tunnel container")
	}
}
