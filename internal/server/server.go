// Package server hosts the speed-test endpoints and the embedded web UI.
package server

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pcamminadi/gospeedtest/web"
)

// Config controls the HTTP server.
type Config struct {
	Addr           string        // e.g. ":8080"
	MaxDownloadMiB int64         // safety cap, default 1024 (1 GiB)
	MaxUploadMiB   int64         // safety cap, default 1024
	ReadTimeout    time.Duration // per request, default 60s — uploads can be large
	IPInfoToken    string        // optional ipinfo.io token for richer ISP data

	// TrustProxyHeaders, when true, makes the server honor
	// X-Forwarded-For / X-Real-IP for client-IP determination. OFF by
	// default to prevent header-spoofing attacks: any direct caller can
	// otherwise force /api/info to issue an outbound ipinfo.io lookup
	// for an IP of their choosing and read the result, draining the
	// operator's quota and acting as an SSRF-style oracle. Only enable
	// this when running behind a reverse proxy that *strips and re-sets*
	// these headers itself.
	TrustProxyHeaders bool
}

func (c *Config) defaults() {
	if c.Addr == "" {
		c.Addr = ":8080"
	}
	if c.MaxDownloadMiB == 0 {
		c.MaxDownloadMiB = 1024
	}
	if c.MaxUploadMiB == 0 {
		c.MaxUploadMiB = 1024
	}
	if c.ReadTimeout == 0 {
		c.ReadTimeout = 60 * time.Second
	}
}

// New returns an *http.Server wired up with the speed-test handlers and the
// embedded web UI. Caller is responsible for ListenAndServe.
func New(cfg Config) *http.Server {
	cfg.defaults()
	mux := http.NewServeMux()

	mux.HandleFunc("/ping", handlePing)
	mux.HandleFunc("/download", handleDownload(cfg))
	mux.HandleFunc("/upload", handleUpload(cfg))
	mux.HandleFunc("/api/info", handleInfo(cfg))
	// /ws is the server-driven ping endpoint. The browser uses it
	// because WebSocket Ping/Pong control frames are processed by the
	// browser's network process without involving the JS event loop —
	// so the server can measure wire RTT accurately even on loopback.
	mux.HandleFunc("/ws", handleWS)
	mux.Handle("/", web.FileServer())

	return &http.Server{
		Addr:    cfg.Addr,
		Handler: withCORSAndNoCache(mux),
		// ReadHeaderTimeout caps the time we'll wait for request headers.
		// Without this, slowloris-style clients can dribble headers for
		// up to ReadTimeout and pin a goroutine + fd per connection. 5s
		// is plenty for any legitimate client; the body still has the
		// full ReadTimeout afterwards.
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       cfg.ReadTimeout,
		// IdleTimeout bounds keep-alive connections so an idle TCP
		// session doesn't hold a goroutine indefinitely.
		IdleTimeout: 60 * time.Second,
		// No WriteTimeout — long downloads must not be cut off mid-stream.
	}
}

// ----- handlers -----

func handlePing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Length", "0")
	w.WriteHeader(http.StatusNoContent)
}

func handleDownload(cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		bytes, err := strconv.ParseInt(r.URL.Query().Get("bytes"), 10, 64)
		if err != nil || bytes <= 0 {
			bytes = 25 << 20 // 25 MiB default
		}
		max := cfg.MaxDownloadMiB << 20
		if bytes > max {
			bytes = max
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", strconv.FormatInt(bytes, 10))
		w.WriteHeader(http.StatusOK)

		// Stream random bytes from crypto/rand directly. crypto/rand is fast
		// enough on modern hardware to saturate gigabit links, and the data
		// is incompressible so intermediate proxies can't cheat.
		_, _ = io.CopyN(w, rand.Reader, bytes)
	}
}

func handleUpload(cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost && r.Method != http.MethodPut {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		max := cfg.MaxUploadMiB << 20
		body := http.MaxBytesReader(w, r.Body, max)
		n, err := io.Copy(io.Discard, body)
		if err != nil && !errors.Is(err, io.EOF) {
			// MaxBytes / client cancellation — still return what we got.
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]int64{"bytes": n})
	}
}

// InfoResponse is what /api/info returns.
type InfoResponse struct {
	ClientIP   string `json:"client_ip"`
	ISP        string `json:"isp,omitempty"`
	City       string `json:"city,omitempty"`
	Region     string `json:"region,omitempty"`
	Country    string `json:"country,omitempty"`
	ServerHost string `json:"server_host"`
	ServerTime string `json:"server_time"`
}

func handleInfo(cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r, cfg.TrustProxyHeaders)
		host, _ := os.Hostname()
		resp := InfoResponse{
			ClientIP:   ip,
			ServerHost: host,
			ServerTime: time.Now().UTC().Format(time.RFC3339),
		}
		// Best-effort ISP lookup; never block the response on failure.
		if isp := lookupIPInfo(ip, cfg.IPInfoToken); isp != nil {
			resp.ISP = isp.Org
			resp.City = isp.City
			resp.Region = isp.Region
			resp.Country = isp.Country
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// clientIP returns the most likely real client IP. When trustProxyHeaders
// is true, X-Forwarded-For (first entry) or X-Real-IP take precedence —
// only safe when the server sits behind a proxy that strips/re-sets those
// headers itself. Otherwise we fall back to the TCP peer address, which
// is the only thing a malicious client cannot forge.
func clientIP(r *http.Request, trustProxyHeaders bool) string {
	if trustProxyHeaders {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			// First entry is the original client.
			if i := strings.IndexByte(xff, ','); i > 0 {
				return strings.TrimSpace(xff[:i])
			}
			return strings.TrimSpace(xff)
		}
		if xr := r.Header.Get("X-Real-IP"); xr != "" {
			return strings.TrimSpace(xr)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ipinfoResponse is the subset of ipinfo.io's response we use.
type ipinfoResponse struct {
	IP      string `json:"ip"`
	City    string `json:"city"`
	Region  string `json:"region"`
	Country string `json:"country"`
	Org     string `json:"org"`
}

// lookupIPInfo asks ipinfo.io about the given IP. Returns nil on any error
// (including private IPs, which the API rejects). Token is optional.
func lookupIPInfo(ip, token string) *ipinfoResponse {
	if ip == "" || isPrivateIP(ip) {
		return nil
	}
	url := fmt.Sprintf("https://ipinfo.io/%s/json", ip)
	if token != "" {
		url += "?token=" + token
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var out ipinfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil
	}
	return &out
}

func isPrivateIP(s string) bool {
	ip := net.ParseIP(s)
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
		return true
	}
	return false
}

// withCORSAndNoCache lets the browser UI (potentially served from a different
// origin during dev) call the API, and prevents caching of speed-test
// responses by intermediate proxies.
func withCORSAndNoCache(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		// Only force no-cache for the speed-test endpoints; static UI assets
		// can still be cached by the browser.
		switch r.URL.Path {
		case "/ping", "/download", "/upload", "/api/info":
			w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	})
}
