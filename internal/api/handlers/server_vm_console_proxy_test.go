package handlers

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestProxyConsoleWebSocketStopsWhenParentContextCancels(t *testing.T) {
	t.Parallel()

	proxyCtx, cancelProxy := context.WithCancel(context.Background())
	defer cancelProxy()

	backendServer, backendClient := net.Pipe()
	defer backendServer.Close()
	defer backendClient.Close()

	errCh := make(chan error, 1)
	upgradedCh := make(chan struct{}, 1)
	upgrader := websocket.Upgrader{
		CheckOrigin: func(*http.Request) bool {
			return true
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			errCh <- err
			return
		}
		defer ws.Close()

		upgradedCh <- struct{}{}
		errCh <- proxyConsoleWebSocket(proxyCtx, ws, backendServer)
	}))
	defer server.Close()

	clientWS, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer clientWS.Close()

	select {
	case <-upgradedCh:
	case err := <-errCh:
		t.Fatalf("proxy returned before cancellation: %v", err)
	case <-time.After(time.Second):
		t.Fatal("websocket upgrade did not complete")
	}

	cancelProxy()

	select {
	case err := <-errCh:
		if !isExpectedConsoleProxyClose(err) {
			t.Fatalf("proxy error = %v, want expected close/cancel error", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("proxy did not stop after parent context cancellation")
	}
}
