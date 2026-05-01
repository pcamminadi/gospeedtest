package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestWSPing(t *testing.T) {
	// Use a tight config so the test runs in ~milliseconds.
	h := handleWSWithConfig(WSPingConfig{
		Samples:     3,
		Spacing:     5 * time.Millisecond,
		PingTimeout: 1 * time.Second,
		MaxLifetime: 5 * time.Second,
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.CloseNow()

	got := []wsPingMessage{}
	for {
		_, data, err := c.Read(ctx)
		if err != nil {
			break
		}
		var m wsPingMessage
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("decode: %v", err)
		}
		got = append(got, m)
		if m.Type == "done" {
			break
		}
	}

	// Expect 3 ping messages + 1 done message.
	if len(got) < 3 {
		t.Fatalf("got %d messages, want at least 3", len(got))
	}
	pings := 0
	for _, m := range got {
		if m.Type == "ping" {
			pings++
			if m.RTTMs < 0 {
				t.Errorf("negative RTT: %v", m.RTTMs)
			}
			// Loopback ping should be well under 100 ms even on a busy CI runner.
			if m.RTTMs > 1000 {
				t.Errorf("RTT %v ms is implausibly large", m.RTTMs)
			}
		}
	}
	if pings != 3 {
		t.Errorf("got %d ping messages, want 3", pings)
	}
	if got[len(got)-1].Type != "done" {
		t.Errorf("last message type = %q, want done", got[len(got)-1].Type)
	}
}

func TestWSEndpointWiredInServer(t *testing.T) {
	srv := New(Config{})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Just verify the route accepts an upgrade.
	c, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.CloseNow()

	// Read at least one message to confirm the loop is running.
	_, _, err = c.Read(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
}
