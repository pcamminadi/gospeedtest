// Package speedtest measures network throughput and latency between the
// caller and a gospeedtest server. The same package is used by the CLI and
// the embedded web client (the web client speaks the same HTTP protocol from
// JavaScript).
//
// Protocol:
//
//	GET  /ping              -> 204 No Content, used for RTT samples.
//	GET  /download?bytes=N  -> N bytes of incompressible random data.
//	POST /upload            -> drains the body, replies with the byte count.
//
// All three are plain HTTP so a stock load balancer or reverse proxy works
// without special handling.
package speedtest

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync/atomic"
	"time"
)

// Config controls a full test run. Zero values get sensible defaults.
type Config struct {
	ServerURL    string        // base URL, e.g. "http://localhost:8080"
	HTTPClient   *http.Client  // optional; one is built if nil
	PingSamples  int           // default 10
	TestDuration time.Duration // per phase, default 10s
	Streams      int           // parallel HTTP streams per phase, default 4
	ChunkBytes   int64         // bytes per request in download/upload, default 25 MiB
	SampleEvery  time.Duration // how often to emit a Sample, default 100ms
}

func (c *Config) defaults() {
	if c.PingSamples == 0 {
		c.PingSamples = 10
	}
	if c.TestDuration == 0 {
		c.TestDuration = 10 * time.Second
	}
	if c.Streams == 0 {
		c.Streams = 4
	}
	if c.ChunkBytes == 0 {
		c.ChunkBytes = 25 << 20 // 25 MiB
	}
	if c.SampleEvery == 0 {
		c.SampleEvery = 100 * time.Millisecond
	}
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{
			// No overall timeout — phases enforce their own duration.
			Transport: &http.Transport{
				MaxIdleConns:        c.Streams * 2,
				MaxIdleConnsPerHost: c.Streams * 2,
				IdleConnTimeout:     30 * time.Second,
				DisableCompression:  true, // we're sending random bytes
			},
		}
	}
}

// Phase identifies which part of the test produced a sample.
type Phase string

const (
	PhasePing     Phase = "ping"
	PhaseDownload Phase = "download"
	PhaseUpload   Phase = "upload"
	PhaseDone     Phase = "done"
)

// Sample is a snapshot emitted while a phase is in progress.
type Sample struct {
	Phase   Phase
	Elapsed time.Duration // time since the phase began
	Bytes   int64         // cumulative bytes for download/upload
	Mbps    float64       // instantaneous bits/sec, megabits
	PingMs  float64       // most-recent RTT for the ping phase
}

// Result is the final summary of one test.
type Result struct {
	PingMs       float64       `json:"ping_ms"`
	JitterMs     float64       `json:"jitter_ms"`
	DownloadMbps float64       `json:"download_mbps"`
	UploadMbps   float64       `json:"upload_mbps"`
	BytesDown    int64         `json:"bytes_down"`
	BytesUp      int64         `json:"bytes_up"`
	Duration     time.Duration `json:"duration"`
	StartedAt    time.Time     `json:"started_at"`
}

// Run executes ping, download and upload sequentially against cfg.ServerURL,
// streaming samples to onSample (which may be nil). It returns the final
// Result or the first error encountered.
func Run(ctx context.Context, cfg Config, onSample func(Sample)) (Result, error) {
	cfg.defaults()
	if cfg.ServerURL == "" {
		return Result{}, errors.New("speedtest: ServerURL is required")
	}
	if onSample == nil {
		onSample = func(Sample) {}
	}

	res := Result{StartedAt: time.Now()}

	pingMs, jitterMs, err := runPing(ctx, cfg, onSample)
	if err != nil {
		return res, fmt.Errorf("ping: %w", err)
	}
	res.PingMs, res.JitterMs = pingMs, jitterMs

	dl, dlBytes, err := runDownload(ctx, cfg, onSample)
	if err != nil {
		return res, fmt.Errorf("download: %w", err)
	}
	res.DownloadMbps, res.BytesDown = dl, dlBytes

	ul, ulBytes, err := runUpload(ctx, cfg, onSample)
	if err != nil {
		return res, fmt.Errorf("upload: %w", err)
	}
	res.UploadMbps, res.BytesUp = ul, ulBytes

	res.Duration = time.Since(res.StartedAt)
	onSample(Sample{Phase: PhaseDone, Elapsed: res.Duration})
	return res, nil
}

// ---------- Ping ----------

func runPing(ctx context.Context, cfg Config, onSample func(Sample)) (avgMs, jitterMs float64, err error) {
	samples := make([]float64, 0, cfg.PingSamples)
	start := time.Now()
	for i := 0; i < cfg.PingSamples; i++ {
		select {
		case <-ctx.Done():
			return 0, 0, ctx.Err()
		default:
		}
		t0 := time.Now()
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, cfg.ServerURL+"/ping", nil)
		resp, err := cfg.HTTPClient.Do(req)
		if err != nil {
			return 0, 0, err
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		ms := float64(time.Since(t0).Microseconds()) / 1000.0
		samples = append(samples, ms)
		onSample(Sample{
			Phase:   PhasePing,
			Elapsed: time.Since(start),
			PingMs:  ms,
		})
		// Spread requests slightly so we don't pummel the server.
		time.Sleep(50 * time.Millisecond)
	}
	return pingStats(samples)
}

func pingStats(samples []float64) (median, jitter float64, err error) {
	if len(samples) == 0 {
		return 0, 0, errors.New("no ping samples")
	}
	sorted := append([]float64(nil), samples...)
	sort.Float64s(sorted)
	median = sorted[len(sorted)/2]

	// Jitter: mean absolute deviation between consecutive samples (RFC 3550-ish).
	if len(samples) > 1 {
		var sum float64
		for i := 1; i < len(samples); i++ {
			d := samples[i] - samples[i-1]
			if d < 0 {
				d = -d
			}
			sum += d
		}
		jitter = sum / float64(len(samples)-1)
	}
	return median, jitter, nil
}

// ---------- Download ----------

func runDownload(ctx context.Context, cfg Config, onSample func(Sample)) (mbps float64, total int64, err error) {
	return runStreamed(ctx, cfg, PhaseDownload, onSample, downloadWorker)
}

func downloadWorker(ctx context.Context, cfg Config, counter *atomic.Int64) error {
	url := fmt.Sprintf("%s/download?bytes=%d", cfg.ServerURL, cfg.ChunkBytes)
	buf := make([]byte, 64<<10)
	for {
		if ctx.Err() != nil {
			return nil
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		resp, err := cfg.HTTPClient.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		for {
			n, rerr := resp.Body.Read(buf)
			if n > 0 {
				counter.Add(int64(n))
			}
			if rerr != nil {
				_ = resp.Body.Close()
				if rerr == io.EOF {
					break
				}
				if ctx.Err() != nil {
					return nil
				}
				return rerr
			}
		}
	}
}

// ---------- Upload ----------

func runUpload(ctx context.Context, cfg Config, onSample func(Sample)) (mbps float64, total int64, err error) {
	return runStreamed(ctx, cfg, PhaseUpload, onSample, uploadWorker)
}

func uploadWorker(ctx context.Context, cfg Config, counter *atomic.Int64) error {
	url := cfg.ServerURL + "/upload"
	for {
		if ctx.Err() != nil {
			return nil
		}
		body := &countingRandReader{remaining: cfg.ChunkBytes, ctx: ctx, counter: counter}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
		if err != nil {
			return err
		}
		req.ContentLength = cfg.ChunkBytes
		req.Header.Set("Content-Type", "application/octet-stream")
		resp, err := cfg.HTTPClient.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}

// countingRandReader emits up to `remaining` random bytes and increments
// counter as it goes, so progress reflects what's actually been put on the
// wire (and not just buffered in user-space).
type countingRandReader struct {
	remaining int64
	ctx       context.Context
	counter   *atomic.Int64
}

func (r *countingRandReader) Read(p []byte) (int, error) {
	if r.ctx.Err() != nil {
		return 0, r.ctx.Err()
	}
	if r.remaining <= 0 {
		return 0, io.EOF
	}
	n := len(p)
	if int64(n) > r.remaining {
		n = int(r.remaining)
	}
	if _, err := rand.Read(p[:n]); err != nil {
		return 0, err
	}
	r.remaining -= int64(n)
	r.counter.Add(int64(n))
	return n, nil
}

// ---------- Shared streaming driver ----------

type workerFn func(ctx context.Context, cfg Config, counter *atomic.Int64) error

// runStreamed launches cfg.Streams workers, samples the cumulative byte
// counter every cfg.SampleEvery, and stops them after cfg.TestDuration. It
// reports the average throughput over a "stable" window (drops the first
// 1.5s ramp-up where TCP is still scaling).
func runStreamed(ctx context.Context, cfg Config, phase Phase, onSample func(Sample), worker workerFn) (float64, int64, error) {
	phaseCtx, cancel := context.WithTimeout(ctx, cfg.TestDuration)
	defer cancel()

	var counter atomic.Int64
	errCh := make(chan error, cfg.Streams)
	for i := 0; i < cfg.Streams; i++ {
		go func() { errCh <- worker(phaseCtx, cfg, &counter) }()
	}

	const rampUp = 1500 * time.Millisecond
	tick := time.NewTicker(cfg.SampleEvery)
	defer tick.Stop()

	start := time.Now()
	var stableStart time.Time
	var stableStartBytes int64

	for {
		select {
		case <-phaseCtx.Done():
			// Drain workers; ignore context-cancellation errors.
			for i := 0; i < cfg.Streams; i++ {
				if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
					return 0, counter.Load(), err
				}
			}
			total := counter.Load()
			if stableStart.IsZero() {
				// Test was shorter than ramp-up; fall back to whole-window average.
				elapsed := time.Since(start).Seconds()
				return bytesToMbps(total, elapsed), total, nil
			}
			elapsed := time.Since(stableStart).Seconds()
			return bytesToMbps(total-stableStartBytes, elapsed), total, nil
		case now := <-tick.C:
			elapsed := now.Sub(start)
			bytes := counter.Load()
			if stableStart.IsZero() && elapsed >= rampUp {
				stableStart = now
				stableStartBytes = bytes
			}
			// Instantaneous Mbps over the last sample window — for live UI feel.
			// We use the slope over a short trailing window.
			onSample(Sample{
				Phase:   phase,
				Elapsed: elapsed,
				Bytes:   bytes,
				Mbps:    bytesToMbps(bytes, elapsed.Seconds()),
			})
		}
	}
}

func bytesToMbps(bytes int64, seconds float64) float64 {
	if seconds <= 0 {
		return 0
	}
	return float64(bytes*8) / seconds / 1_000_000
}
