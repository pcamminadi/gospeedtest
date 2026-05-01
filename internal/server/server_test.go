package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestClientIP(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string]string
		remote  string
		want    string
	}{
		{
			name:   "RemoteAddr fallback strips port",
			remote: "203.0.113.7:54321",
			want:   "203.0.113.7",
		},
		{
			name:    "X-Real-IP wins over RemoteAddr",
			headers: map[string]string{"X-Real-IP": "198.51.100.4"},
			remote:  "10.0.0.1:1",
			want:    "198.51.100.4",
		},
		{
			name:    "X-Forwarded-For wins, first entry used",
			headers: map[string]string{"X-Forwarded-For": "203.0.113.5, 10.0.0.1, 10.0.0.2"},
			remote:  "10.0.0.99:1",
			want:    "203.0.113.5",
		},
		{
			name:    "X-Forwarded-For single entry",
			headers: map[string]string{"X-Forwarded-For": "203.0.113.5"},
			remote:  "10.0.0.99:1",
			want:    "203.0.113.5",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/info", nil)
			r.RemoteAddr = tc.remote
			for k, v := range tc.headers {
				r.Header.Set(k, v)
			}
			if got := clientIP(r); got != tc.want {
				t.Errorf("clientIP = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestIsPrivateIP(t *testing.T) {
	cases := []struct {
		ip      string
		private bool
	}{
		{"127.0.0.1", true},
		{"::1", true},
		{"10.0.0.1", true},
		{"192.168.1.1", true},
		{"172.16.5.4", true},
		{"169.254.1.1", true},
		{"0.0.0.0", true},
		{"", true},        // unparseable
		{"not-an-ip", true}, // unparseable
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"203.0.113.5", false},
	}
	for _, tc := range cases {
		if got := isPrivateIP(tc.ip); got != tc.private {
			t.Errorf("isPrivateIP(%q) = %v, want %v", tc.ip, got, tc.private)
		}
	}
}

func TestHandlePing(t *testing.T) {
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ping", nil)
	handlePing(rr, r)
	if rr.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rr.Code)
	}
	if got := rr.Header().Get("Content-Length"); got != "0" {
		t.Errorf("Content-Length = %q, want 0", got)
	}
}

func TestHandlePingMethodNotAllowed(t *testing.T) {
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/ping", nil)
	handlePing(rr, r)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rr.Code)
	}
}

func TestHandleDownload(t *testing.T) {
	cfg := Config{}
	cfg.defaults()
	h := handleDownload(cfg)

	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/download?bytes=4096", nil)
	h(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if got := rr.Header().Get("Content-Length"); got != "4096" {
		t.Errorf("Content-Length = %q, want 4096", got)
	}
	if rr.Body.Len() != 4096 {
		t.Errorf("body len = %d, want 4096", rr.Body.Len())
	}
}

func TestHandleDownloadCapsLargeRequests(t *testing.T) {
	cfg := Config{MaxDownloadMiB: 1} // 1 MiB cap
	cfg.defaults()
	h := handleDownload(cfg)

	rr := httptest.NewRecorder()
	// Ask for 100 MiB; should be capped to 1 MiB.
	r := httptest.NewRequest(http.MethodGet, "/download?bytes="+strconv.Itoa(100<<20), nil)
	h(rr, r)

	wantBytes := int64(1 << 20)
	if got, _ := strconv.ParseInt(rr.Header().Get("Content-Length"), 10, 64); got != wantBytes {
		t.Errorf("Content-Length = %d, want %d", got, wantBytes)
	}
	if int64(rr.Body.Len()) != wantBytes {
		t.Errorf("body len = %d, want %d", rr.Body.Len(), wantBytes)
	}
}

func TestHandleUpload(t *testing.T) {
	cfg := Config{}
	cfg.defaults()
	h := handleUpload(cfg)

	body := bytes.Repeat([]byte("x"), 8192)
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/upload", bytes.NewReader(body))
	h(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var resp map[string]int64
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["bytes"] != int64(len(body)) {
		t.Errorf("bytes = %d, want %d", resp["bytes"], len(body))
	}
}

// TestServerEndToEnd boots the actual New() server on an httptest listener
// and verifies the routes wire up correctly.
func TestServerEndToEnd(t *testing.T) {
	srv := New(Config{})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	// /ping
	resp, err := http.Get(ts.URL + "/ping")
	if err != nil {
		t.Fatalf("ping: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("ping status = %d, want 204", resp.StatusCode)
	}

	// /download with explicit byte count
	resp, err = http.Get(ts.URL + "/download?bytes=2048")
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if len(body) != 2048 {
		t.Errorf("download body = %d bytes, want 2048", len(body))
	}

	// /upload echoes byte count
	resp, err = http.Post(ts.URL+"/upload", "application/octet-stream", strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	defer resp.Body.Close()
	var ur map[string]int64
	if err := json.NewDecoder(resp.Body).Decode(&ur); err != nil {
		t.Fatalf("upload decode: %v", err)
	}
	if ur["bytes"] != 5 {
		t.Errorf("upload bytes = %d, want 5", ur["bytes"])
	}

	// /api/info — local connection, ISP fields should be empty (private IP).
	resp, err = http.Get(ts.URL + "/api/info")
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	defer resp.Body.Close()
	var info InfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		t.Fatalf("info decode: %v", err)
	}
	if info.ServerHost == "" {
		t.Errorf("ServerHost is empty")
	}
	if info.ClientIP == "" {
		t.Errorf("ClientIP is empty")
	}
}

func TestCORSPreflight(t *testing.T) {
	srv := New(Config{})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodOptions, ts.URL+"/ping", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("options: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("OPTIONS status = %d, want 204", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("ACAO = %q, want *", got)
	}
}
