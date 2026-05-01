package server

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/coder/websocket"
)

// WSPingConfig controls the /ws ping loop. Defaults are fine for the
// browser UI; callers can override for tests.
type WSPingConfig struct {
	// Samples is the number of ping/pong round-trips to perform.
	Samples int
	// Spacing is the delay between successive pings.
	Spacing time.Duration
	// PingTimeout bounds an individual ping/pong. Treated as a hard error
	// if exceeded — typically only happens when the network or peer is
	// gone.
	PingTimeout time.Duration
	// MaxLifetime caps the entire connection. Defends against clients
	// that open the WS and stop reading.
	MaxLifetime time.Duration
}

func (c *WSPingConfig) defaults() {
	if c.Samples == 0 {
		c.Samples = 10
	}
	if c.Spacing == 0 {
		c.Spacing = 100 * time.Millisecond
	}
	if c.PingTimeout == 0 {
		c.PingTimeout = 3 * time.Second
	}
	if c.MaxLifetime == 0 {
		c.MaxLifetime = 30 * time.Second
	}
}

// wsPingMessage is the JSON shape we stream to the client between pings.
type wsPingMessage struct {
	Type  string  `json:"type"`            // "ping" or "done"
	Seq   int     `json:"seq,omitempty"`   // 0..Samples-1
	RTTMs float64 `json:"rtt_ms,omitempty"`
}

// handleWS upgrades to a WebSocket and runs server-driven ping
// measurements. The server's nanosecond clock (Go's monotonic time) is
// the time source — there's no clock skew between participants because
// only the server measures.
//
// Why this is more accurate than fetch()+performance.now() in the browser:
//
//	1. Browsers clamp `performance.now()` and Resource Timing entries
//	   for Spectre mitigation (1 ms in Firefox/Safari, ~100 µs in
//	   Chrome). The server's time is unconstrained.
//	2. WebSocket Ping/Pong control frames (RFC 6455 §5.5.2) are handled
//	   by the browser's network process at the protocol level, *without
//	   involving the JS event loop or the renderer process*. That
//	   eliminates the IPC + microtask hop that fetch() pays per call.
//
// The result is wire-RTT-accurate even on loopback.
func handleWSWithConfig(cfg WSPingConfig) http.HandlerFunc {
	cfg.defaults()
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			// We already serve `Access-Control-Allow-Origin: *` for
			// the speed-test endpoints; mirror that here so the UI
			// still works if it's served from a different origin.
			OriginPatterns:     []string{"*"},
			InsecureSkipVerify: true,
		})
		if err != nil {
			return
		}
		// Bound the connection lifetime so a misbehaving client can't
		// pin a goroutine forever.
		ctx, cancel := context.WithTimeout(r.Context(), cfg.MaxLifetime)
		defer cancel()
		defer c.CloseNow()

		// Drain the read side so the library's frame loop runs — that's
		// what processes incoming Pong frames and unblocks c.Ping. We
		// don't expect application messages from the client; if any
		// arrive we simply discard them.
		readErr := make(chan error, 1)
		go func() {
			for {
				_, _, err := c.Read(ctx)
				if err != nil {
					readErr <- err
					return
				}
			}
		}()

		for i := 0; i < cfg.Samples; i++ {
			if ctx.Err() != nil {
				return
			}

			pingCtx, pingCancel := context.WithTimeout(ctx, cfg.PingTimeout)
			t0 := time.Now()
			err := c.Ping(pingCtx)
			elapsed := time.Since(t0)
			pingCancel()
			if err != nil {
				// Client disconnected or the pong timed out; stop.
				return
			}

			msg := wsPingMessage{
				Type:  "ping",
				Seq:   i,
				RTTMs: float64(elapsed.Microseconds()) / 1000.0,
			}
			if err := writeJSON(ctx, c, msg); err != nil {
				return
			}

			// Pace the loop. We want roughly cfg.Spacing between
			// pings, but if the previous ping itself was slow we
			// don't want to compound that.
			remaining := cfg.Spacing - elapsed
			if remaining > 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(remaining):
				}
			}
		}

		_ = writeJSON(ctx, c, wsPingMessage{Type: "done"})
		_ = c.Close(websocket.StatusNormalClosure, "")
	}
}

// handleWS uses the default WSPingConfig.
var handleWS = handleWSWithConfig(WSPingConfig{})

func writeJSON(ctx context.Context, c *websocket.Conn, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.Write(ctx, websocket.MessageText, b)
}
