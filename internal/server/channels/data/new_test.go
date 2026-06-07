package data

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-idp/inlets/internal/client"
	"github.com/go-idp/inlets/internal/server/types"
	"github.com/gorilla/websocket"
)

// stub tunnel container: only Get is used for data channel open + ping/pong.
type testTunnelCont struct{ m *types.TunnelMapping }

func (c *testTunnelCont) Create(string, types.GetToken, *websocket.Conn, *client.Authentication, *sync.Mutex) {
}
func (c *testTunnelCont) Get(id string) *types.TunnelMapping {
	if id == "ct1" {
		return c.m
	}
	return nil
}
func (c *testTunnelCont) Set(string, string, interface{}) error { return nil }
func (c *testTunnelCont) Remove(string)                        {}
func (c *testTunnelCont) RegisterRequest(string, string, *net.Conn) {}
func (c *testTunnelCont) ConnectRequest(string, string, *net.Conn) error { return nil }
func (c *testTunnelCont) ListAll() map[string]*types.TunnelMapping {
	if c.m == nil {
		return map[string]*types.TunnelMapping{}
	}
	return map[string]*types.TunnelMapping{"ct1": c.m}
}

func TestDataChannelJSONPingReturnsPong(t *testing.T) {
	dm := &sync.RWMutex{}
	tm := &types.TunnelMapping{
		ClientId:      "cl1",
		DataMu:        dm,
		DataSockets:   make(map[string]*websocket.Conn),
		DataWriteMu:   make(map[string]*sync.Mutex),
		UseNewProtocol: true,
	}
	ctx := &types.Context{
		Container: &testTunnelCont{m: tm},
	}
	h := NewDataChannelHandler(ctx)

	srv := httptest.NewServer(http.HandlerFunc(h.HandleConnection))
	defer srv.Close()

	u := "ws" + strings.TrimPrefix(srv.URL, "http") + "?clientId=cl1&containerId=ct1&streamId=s1"
	conn, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	ping, _ := json.Marshal([]interface{}{"ping", nil})
	if err := conn.WriteMessage(websocket.TextMessage, ping); err != nil {
		t.Fatalf("write ping: %v", err)
	}
	mt, body, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if mt != websocket.TextMessage {
		t.Fatalf("message type: got %d want Text", mt)
	}
	var arr []interface{}
	if err := json.Unmarshal(body, &arr); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(arr) < 1 {
		t.Fatal("empty json array")
	}
	if s, _ := arr[0].(string); s != "pong" {
		t.Fatalf("event: got %q want pong", s)
	}
}

func TestDataChannelJSONPingWithAnonymousClientId(t *testing.T) {
	// When monitor assigns anonymous-* and persists it on the tunnel, /_/data must
	// accept the same id (regression: empty container ClientId + anonymous in query = 403).
	anonID := "anonymous-01020304"
	dm := &sync.RWMutex{}
	tm := &types.TunnelMapping{
		ClientId:       anonID,
		DataMu:         dm,
		DataSockets:    make(map[string]*websocket.Conn),
		DataWriteMu:    make(map[string]*sync.Mutex),
		UseNewProtocol: true,
	}
	ctx := &types.Context{
		Container: &testTunnelCont{m: tm},
	}
	h := NewDataChannelHandler(ctx)

	srv := httptest.NewServer(http.HandlerFunc(h.HandleConnection))
	defer srv.Close()

	u := "ws" + strings.TrimPrefix(srv.URL, "http") + "?clientId=" + anonID + "&containerId=ct1&streamId=s1"
	conn, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	ping, _ := json.Marshal([]interface{}{"ping", nil})
	if err := conn.WriteMessage(websocket.TextMessage, ping); err != nil {
		t.Fatalf("write ping: %v", err)
	}
	mt, body, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if mt != websocket.TextMessage {
		t.Fatalf("message type: got %d want Text", mt)
	}
	var arr []interface{}
	if err := json.Unmarshal(body, &arr); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(arr) < 1 {
		t.Fatal("empty json array")
	}
	if s, _ := arr[0].(string); s != "pong" {
		t.Fatalf("event: got %q want pong", s)
	}
}
