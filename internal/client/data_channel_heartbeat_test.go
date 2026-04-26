package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestDataChannelSendPingWithNoConnectionStopsHeartbeat(t *testing.T) {
	c := New(&Options{})
	c.logger = log.New(io.Discard, "", 0)
	c.dataPingTimer["x"] = time.AfterFunc(time.Hour, func() {})
	if len(c.dataPingTimer) != 1 {
		t.Fatalf("precondition: timer map")
	}
	c.sendDataChannelPing("missing")
	c.dataHeartbeatMu.Lock()
	_, hasPing := c.dataPingTimer["missing"]
	_, hasTO := c.dataPingTimeoutTimer["missing"]
	c.dataHeartbeatMu.Unlock()
	if hasPing {
		t.Fatal("expected ping timer cleared for missing stream")
	}
	if hasTO {
		t.Fatal("expected no timeout for missing stream after sendDataChannelPing")
	}
	if tm := c.dataPingTimer["x"]; tm != nil {
		tm.Stop()
	}
	delete(c.dataPingTimer, "x")
}

func TestDataChannelHandleDataChannelPongRestartsTimer(t *testing.T) {
	c := New(&Options{})
	c.pingInterval = 80 * time.Millisecond
	c.pingTimeout = 5 * time.Second
	c.dataPingTimeoutTimer["s"] = time.AfterFunc(time.Hour, func() {
		panic("pong should cancel the outstanding data ping timeout")
	})
	c.handleDataChannelPong("s")
	if c.dataPingTimeoutTimer["s"] != nil {
		t.Fatal("expected timeout entry cleared (timer stopped and deleted)")
	}
	c.dataHeartbeatMu.Lock()
	tmr := c.dataPingTimer["s"]
	c.dataHeartbeatMu.Unlock()
	if tmr == nil {
		t.Fatal("expected next ping to be scheduled")
	}
	if !tmr.Stop() {
		t.Log("next ping timer already ran (slow CI); skipping Stop check")
	}
}

func TestDataChannelHeartbeatTwoApplicationPings(t *testing.T) {
	var pingCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
		sc, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer sc.Close()
		for {
			mt, b, err := sc.ReadMessage()
			if err != nil {
				return
			}
			if mt != websocket.TextMessage {
				continue
			}
			var arr []interface{}
			if err := json.Unmarshal(b, &arr); err != nil {
				continue
			}
			if len(arr) < 1 {
				continue
			}
			s, _ := arr[0].(string)
			if s != "ping" {
				continue
			}
			n := atomic.AddInt32(&pingCount, 1)
			pb, _ := json.Marshal([]interface{}{"pong", nil})
			_ = sc.WriteMessage(websocket.TextMessage, pb)
			if n >= 2 {
				return
			}
		}
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/data"
	c := New(&Options{})
	c.logger = log.New(io.Discard, "", 0)
	c.pingInterval = 30 * time.Millisecond
	c.pingTimeout = 2 * time.Second

	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	clientConn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	sid := "stream-e2e"
	c.dataConnMu.Lock()
	c.dataConns[sid] = clientConn
	c.dataWriteMu[sid] = &sync.Mutex{}
	c.dataConnMu.Unlock()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		c.handleDataChannel(sid, clientConn)
	}()

	deadline := time.Now().Add(5 * time.Second)
	for atomic.LoadInt32(&pingCount) < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if n := atomic.LoadInt32(&pingCount); n < 2 {
		_ = clientConn.Close()
		wg.Wait()
		t.Fatalf("expected 2 app-layer pings, got %d", n)
	}

	_ = clientConn.Close()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("handleDataChannel did not exit after close")
	}
}

func TestDataChannelPongInResponseToServerPing(t *testing.T) {
	// One reader on the server: reply to the client heartbeat ping, then send a server ping
	// and expect a JSON text pong (sendDataChannelPong).
	errc := make(chan error, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
		sc, err := up.Upgrade(w, r, nil)
		if err != nil {
			errc <- err
			return
		}
		defer sc.Close()
		mt, b, err := sc.ReadMessage()
		if err != nil {
			errc <- err
			return
		}
		if mt != websocket.TextMessage {
			errc <- fmt.Errorf("first frame: want TextMessage, got %d", mt)
			return
		}
		var a1 []interface{}
		if err := json.Unmarshal(b, &a1); err != nil {
			errc <- err
			return
		}
		if len(a1) < 1 {
			errc <- fmt.Errorf("first frame: empty")
			return
		}
		if s, _ := a1[0].(string); s != "ping" {
			errc <- fmt.Errorf("first frame: want app ping, got %v", a1[0])
			return
		}
		pb, _ := json.Marshal([]interface{}{"pong", nil})
		if err := sc.WriteMessage(websocket.TextMessage, pb); err != nil {
			errc <- err
			return
		}
		sPing, _ := json.Marshal([]interface{}{"ping", nil})
		if err := sc.WriteMessage(websocket.TextMessage, sPing); err != nil {
			errc <- err
			return
		}
		mt, resp, err := sc.ReadMessage()
		if err != nil {
			errc <- err
			return
		}
		if mt != websocket.TextMessage {
			errc <- fmt.Errorf("reply frame: want TextMessage, got %d", mt)
			return
		}
		var a2 []interface{}
		if err := json.Unmarshal(resp, &a2); err != nil {
			errc <- err
			return
		}
		if len(a2) < 1 {
			errc <- fmt.Errorf("reply frame: empty")
			return
		}
		if s, _ := a2[0].(string); s != "pong" {
			errc <- fmt.Errorf("reply frame: want app pong, got %v", a2[0])
			return
		}
		errc <- nil
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	c := New(&Options{})
	c.logger = log.New(io.Discard, "", 0)
	c.pingInterval = time.Hour
	c.pingTimeout = time.Hour

	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	clientConn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	sid := "stream-srv-ping"
	c.dataConnMu.Lock()
	c.dataConns[sid] = clientConn
	c.dataWriteMu[sid] = &sync.Mutex{}
	c.dataConnMu.Unlock()
	go c.handleDataChannel(sid, clientConn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	select {
	case err := <-errc:
		_ = clientConn.Close()
		if err != nil {
			t.Fatalf("server: %v", err)
		}
	case <-ctx.Done():
		_ = clientConn.Close()
		t.Fatal("timeout waiting for server to finish")
	}
}
