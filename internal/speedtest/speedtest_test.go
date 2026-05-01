package speedtest

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func TestPingStats(t *testing.T) {
	tests := []struct {
		name      string
		in        []float64
		median    float64
		jitter    float64
		expectErr bool
	}{
		{
			name:      "empty errors",
			in:        nil,
			expectErr: true,
		},
		{
			name:   "single sample no jitter",
			in:     []float64{42},
			median: 42,
			jitter: 0,
		},
		{
			name:   "constant samples",
			in:     []float64{10, 10, 10, 10},
			median: 10,
			jitter: 0,
		},
		{
			name:   "ascending — jitter is mean abs delta",
			in:     []float64{10, 12, 15, 14},
			median: 14, // sorted=[10,12,14,15]; median = sorted[2] = 14
			jitter: (2 + 3 + 1) / 3.0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			med, jit, err := pingStats(tc.in)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if math.Abs(med-tc.median) > 1e-9 {
				t.Errorf("median: got %v want %v", med, tc.median)
			}
			if math.Abs(jit-tc.jitter) > 1e-9 {
				t.Errorf("jitter: got %v want %v", jit, tc.jitter)
			}
		})
	}
}

func TestBytesToMbps(t *testing.T) {
	tests := []struct {
		name    string
		bytes   int64
		seconds float64
		want    float64
	}{
		{"zero seconds is zero", 1024, 0, 0},
		{"negative seconds is zero", 1024, -1, 0},
		{"125000 bytes per second is 1 Mbps", 125_000, 1, 1},
		{"1 GB in 8 seconds is 1000 Mbps (1 Gbps)", 1_000_000_000, 8, 1000},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := bytesToMbps(tc.bytes, tc.seconds)
			if math.Abs(got-tc.want) > 1e-6 {
				t.Errorf("bytesToMbps(%d, %v) = %v, want %v", tc.bytes, tc.seconds, got, tc.want)
			}
		})
	}
}

func TestCountingRandReader(t *testing.T) {
	var counter atomic.Int64
	r := &countingRandReader{
		remaining: 1024,
		ctx:       context.Background(),
		counter:   &counter,
	}
	got, err := io.Copy(io.Discard, r)
	if err != nil {
		t.Fatalf("copy: %v", err)
	}
	if got != 1024 {
		t.Errorf("read %d bytes, want 1024", got)
	}
	if counter.Load() != 1024 {
		t.Errorf("counter = %d, want 1024", counter.Load())
	}
}

func TestCountingRandReaderRespectsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var counter atomic.Int64
	r := &countingRandReader{
		remaining: 1024,
		ctx:       ctx,
		counter:   &counter,
	}
	buf := make([]byte, 64)
	n, err := r.Read(buf)
	if n != 0 {
		t.Errorf("read %d bytes after cancel, want 0", n)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

// fakeServer mimics the gospeedtest endpoints just well enough for the
// speedtest client to drive a full Run() in-process.
func fakeServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/download", func(w http.ResponseWriter, r *http.Request) {
		bytes, _ := strconv.ParseInt(r.URL.Query().Get("bytes"), 10, 64)
		if bytes <= 0 {
			bytes = 64 * 1024
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", strconv.FormatInt(bytes, 10))
		_, _ = io.CopyN(w, rand.Reader, bytes)
	})
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	})
	return httptest.NewServer(mux)
}

func TestRunEndToEnd(t *testing.T) {
	srv := fakeServer(t)
	defer srv.Close()

	cfg := Config{
		ServerURL:    srv.URL,
		PingSamples:  3,
		TestDuration: 600 * time.Millisecond,
		Streams:      2,
		ChunkBytes:   256 << 10, // 256 KiB
		SampleEvery:  100 * time.Millisecond,
	}

	var samples int
	res, err := Run(context.Background(), cfg, func(s Sample) {
		samples++
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.PingMs <= 0 {
		t.Errorf("PingMs = %v, want > 0", res.PingMs)
	}
	if res.DownloadMbps <= 0 {
		t.Errorf("DownloadMbps = %v, want > 0", res.DownloadMbps)
	}
	if res.UploadMbps <= 0 {
		t.Errorf("UploadMbps = %v, want > 0", res.UploadMbps)
	}
	if res.BytesDown == 0 {
		t.Errorf("BytesDown = 0, want > 0")
	}
	if res.BytesUp == 0 {
		t.Errorf("BytesUp = 0, want > 0")
	}
	if samples == 0 {
		t.Errorf("no samples emitted")
	}
}

func TestRunRequiresServerURL(t *testing.T) {
	_, err := Run(context.Background(), Config{}, nil)
	if err == nil {
		t.Fatal("expected error when ServerURL is empty")
	}
}

func TestRunRespectsContext(t *testing.T) {
	srv := fakeServer(t)
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already canceled

	_, err := Run(ctx, Config{
		ServerURL:    srv.URL,
		PingSamples:  3,
		TestDuration: 100 * time.Millisecond,
	}, nil)
	if err == nil {
		t.Fatal("expected error from canceled context")
	}
}
