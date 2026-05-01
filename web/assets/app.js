(() => {
  "use strict";

  // Tunables — match the defaults in internal/speedtest.
  const CFG = {
    pingSamples:   10,
    testDuration:  10_000, // ms per phase
    streams:       4,
    chunkBytes:    25 * 1024 * 1024, // 25 MiB per request
    sampleEvery:   100,
    rampUpMs:      1500,
    // Gauge max scales dynamically per phase so a 50 Mbps line and a 1 Gbps
    // line both look interesting. These are the starting ceilings.
    gaugeMaxStart: 100,
  };

  // ----------- DOM -----------
  const $ = (id) => document.getElementById(id);
  const ui = {
    go:          $("go"),
    phase:       $("phase-label"),
    value:       $("value"),
    needle:      $("needle"),
    arc:         $("arc-fg"),
    ticks:       $("ticks"),
    gaugeWrap:   document.querySelector(".gauge-wrap"),
    ping:        $("ping-value"),
    jitter:      $("jitter-value"),
    download:    $("download-value"),
    downloadSub: $("download-bytes"),
    upload:      $("upload-value"),
    uploadSub:   $("upload-bytes"),
    infoIP:      $("info-ip"),
    infoISP:     $("info-isp"),
    infoLoc:     $("info-loc"),
    infoServer:  $("info-server"),
    serverHost:  $("server-host"),
    rPing:       document.querySelector('.result[data-phase="ping"]'),
    rDown:       document.querySelector('.result[data-phase="download"]'),
    rUp:         document.querySelector('.result[data-phase="upload"]'),
  };

  // Build the static tick marks once. The arc spans -135° to +135° (270°).
  // We draw 9 major ticks (0..max) and small ticks between.
  function buildTicks() {
    const cx = 200, cy = 200, rOut = 178, rMaj = 162, rMin = 168, rLbl = 145;
    const start = -225; // svg rotates clockwise from positive x; this puts 0 at lower-left
    const sweep = 270;
    let svg = "";
    for (let i = 0; i <= 40; i++) {
      const a = (start + (sweep * i) / 40) * Math.PI / 180;
      const isMaj = i % 5 === 0;
      const r1 = rOut;
      const r2 = isMaj ? rMaj : rMin;
      const x1 = cx + r1 * Math.cos(a), y1 = cy + r1 * Math.sin(a);
      const x2 = cx + r2 * Math.cos(a), y2 = cy + r2 * Math.sin(a);
      svg += `<line class="${isMaj ? "major" : ""}" x1="${x1.toFixed(1)}" y1="${y1.toFixed(1)}" x2="${x2.toFixed(1)}" y2="${y2.toFixed(1)}"/>`;
    }
    ui.ticks.innerHTML = svg;
  }

  // Set the needle and arc to a fraction [0..1].
  // We drive the needle via CSS `transform` (style property) rather than the
  // SVG `transform` attribute — CSS transitions don't reliably interpolate
  // SVG presentation-attribute changes across browsers, and that produced
  // the "needle bugging through the page" symptom. CSS-side transforms
  // animate cleanly with the .needle CSS rule.
  const ARC_LEN = 754; // matches stroke-dasharray in HTML
  function setGauge(frac) {
    const f = Math.max(0, Math.min(1, frac));
    const deg = -135 + f * 270; // arc spans -135° to +135° (270° sweep)
    ui.needle.style.transform = `rotate(${deg.toFixed(2)}deg)`;
    ui.arc.setAttribute("stroke-dashoffset", String(ARC_LEN * (1 - f)));
  }

  function setReadout(phaseLabel, value, unit) {
    ui.phase.textContent = phaseLabel;
    ui.value.textContent = value;
    document.querySelector(".unit").textContent = unit;
  }

  function activeResult(which) {
    [ui.rPing, ui.rDown, ui.rUp].forEach(el => el.classList.remove("active"));
    if (which === "ping")     ui.rPing.classList.add("active");
    if (which === "download") ui.rDown.classList.add("active");
    if (which === "upload")   ui.rUp.classList.add("active");
  }

  function fmtBytes(n) {
    if (n < 1024) return `${n} B`;
    if (n < 1024 * 1024) return `${(n/1024).toFixed(1)} KiB`;
    if (n < 1024 * 1024 * 1024) return `${(n/1024/1024).toFixed(1)} MiB`;
    return `${(n/1024/1024/1024).toFixed(2)} GiB`;
  }

  // ----------- Speed test client -----------

  async function fetchInfo() {
    try {
      const r = await fetch("/api/info", { cache: "no-store" });
      const j = await r.json();
      ui.infoIP.textContent     = j.client_ip || "—";
      ui.infoISP.textContent    = j.isp || "(unknown — private network)";
      const loc = [j.city, j.region, j.country].filter(Boolean).join(", ");
      ui.infoLoc.textContent    = loc || "—";
      ui.infoServer.textContent = j.server_host || "—";
      ui.serverHost.textContent = j.server_host ? `server: ${j.server_host}` : "";
    } catch (e) {
      // best-effort only
    }
  }

  // ----------- Ping -----------
  //
  // We prefer the WebSocket /ws endpoint over HTTP /ping in the browser:
  // the *server* measures the round-trip using WebSocket Ping/Pong
  // control frames, which the browser's network process answers
  // automatically without involving the JS event loop. That removes the
  // per-request JS overhead and the timer-precision clamping that
  // browsers apply for Spectre mitigation — server-side measurement uses
  // Go's nanosecond-resolution monotonic clock.
  //
  // If WS fails (older server without /ws, blocked upgrade, proxy
  // stripping the upgrade headers, etc.) we fall back to the HTTP
  // approach using the Resource Timing API.

  async function runPing() {
    activeResult("ping");
    setReadout("Ping", "0", "ms");
    try {
      return await runPingWS();
    } catch (e) {
      console.warn("WS ping unavailable, falling back to HTTP:", e.message || e);
      return await runPingHTTP();
    }
  }

  function runPingWS() {
    return new Promise((resolve, reject) => {
      const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
      const ws = new WebSocket(`${proto}//${window.location.host}/ws`);

      const samples = [];
      let settled = false;
      const settle = (fn, val) => { if (!settled) { settled = true; fn(val); } };

      // Hard ceiling so a stuck connection doesn't hang the UI.
      const timer = setTimeout(() => {
        try { ws.close(); } catch (_) {}
        settle(reject, new Error("ws ping timeout"));
      }, 15000);

      ws.onmessage = (ev) => {
        let msg;
        try { msg = JSON.parse(ev.data); } catch (_) { return; }
        if (msg.type === "ping" && typeof msg.rtt_ms === "number") {
          samples.push(msg.rtt_ms);
          ui.ping.textContent = msg.rtt_ms.toFixed(2);
          setReadout("Ping", msg.rtt_ms.toFixed(2), "ms");
          setGauge(1 - Math.min(msg.rtt_ms, 200) / 200);
        } else if (msg.type === "done") {
          finalize();
        }
      };

      ws.onerror = () => {
        // Errors fire for both connect failures and mid-stream issues.
        // If we already collected samples, still try to use them.
        if (samples.length === 0) {
          clearTimeout(timer);
          settle(reject, new Error("ws connection error"));
        }
      };

      ws.onclose = () => {
        clearTimeout(timer);
        finalize();
      };

      function finalize() {
        if (settled) return;
        if (samples.length === 0) {
          settle(reject, new Error("ws closed with no samples"));
          return;
        }
        try { ws.close(); } catch (_) {}
        const sorted = [...samples].sort((a, b) => a - b);
        const median = sorted[Math.floor(sorted.length / 2)];
        let jitter = 0;
        for (let i = 1; i < samples.length; i++) {
          jitter += Math.abs(samples[i] - samples[i - 1]);
        }
        jitter = samples.length > 1 ? jitter / (samples.length - 1) : 0;
        ui.ping.textContent   = median.toFixed(2);
        ui.jitter.textContent = jitter.toFixed(2);
        settle(resolve, { ping: median, jitter });
      }
    });
  }

  // measureOnePingHTTP performs one round-trip and returns its duration in
  // milliseconds, preferring the Performance Resource Timing API over
  // performance.now() brackets so we exclude JS scheduling overhead.
  async function measureOnePingHTTP(seq) {
    const url = `/ping?n=${seq}_${Math.random().toString(36).slice(2, 10)}`;
    const t0 = performance.now();
    await fetch(url, { cache: "no-store" });
    const wallMs = performance.now() - t0;
    try {
      const fullURL = new URL(url, window.location.href).href;
      const entries = performance.getEntriesByName(fullURL);
      const e = entries[entries.length - 1];
      if (e && e.responseStart > 0 && e.requestStart > 0) {
        return e.responseStart - e.requestStart;
      }
    } catch (_) { /* fall through to wall-clock */ }
    return wallMs;
  }

  async function runPingHTTP() {
    // Warm-up: prime the TCP/HTTP connection so the first counted sample
    // doesn't include handshake overhead. Discarded.
    try { await fetch("/ping", { cache: "no-store" }); } catch (_) {}

    const samples = [];
    for (let i = 0; i < CFG.pingSamples; i++) {
      let ms;
      try { ms = await measureOnePingHTTP(i); } catch (_) { continue; }
      samples.push(ms);
      ui.ping.textContent = ms.toFixed(2);
      setReadout("Ping", ms.toFixed(2), "ms");
      setGauge(1 - Math.min(ms, 200) / 200);
      await sleep(50);
    }

    if (typeof performance.clearResourceTimings === "function") {
      performance.clearResourceTimings();
    }
    if (samples.length === 0) throw new Error("ping failed");

    const sorted = [...samples].sort((a, b) => a - b);
    const median = sorted[Math.floor(sorted.length / 2)];
    let jitter = 0;
    for (let i = 1; i < samples.length; i++) {
      jitter += Math.abs(samples[i] - samples[i - 1]);
    }
    jitter = samples.length > 1 ? jitter / (samples.length - 1) : 0;
    ui.ping.textContent   = median.toFixed(2);
    ui.jitter.textContent = jitter.toFixed(2);
    return { ping: median, jitter };
  }

  async function runDownload() {
    activeResult("download");
    setReadout("Download", "0.00", "Mbps");
    const ctrl = new AbortController();
    const start = performance.now();
    let bytes = 0;
    let stableStart = 0, stableStartBytes = 0;
    let gaugeMax = CFG.gaugeMaxStart;

    const workers = [];
    for (let i = 0; i < CFG.streams; i++) {
      workers.push((async () => {
        while (performance.now() - start < CFG.testDuration && !ctrl.signal.aborted) {
          let resp;
          try {
            resp = await fetch(`/download?bytes=${CFG.chunkBytes}`, {
              cache: "no-store",
              signal: ctrl.signal,
            });
          } catch (e) { return; }
          const reader = resp.body.getReader();
          while (true) {
            const { value, done } = await reader.read();
            if (done) break;
            bytes += value.byteLength;
            if (performance.now() - start >= CFG.testDuration) {
              try { reader.cancel(); } catch (_) {}
              return;
            }
          }
        }
      })());
    }

    // Live updater
    const ticker = setInterval(() => {
      const elapsedMs = performance.now() - start;
      if (stableStart === 0 && elapsedMs >= CFG.rampUpMs) {
        stableStart = performance.now();
        stableStartBytes = bytes;
      }
      const refStart = stableStart || start;
      const refBytes = stableStart ? bytes - stableStartBytes : bytes;
      const seconds  = (performance.now() - refStart) / 1000;
      const mbps = seconds > 0 ? (refBytes * 8) / seconds / 1_000_000 : 0;
      while (mbps > gaugeMax * 0.85) gaugeMax *= 2;
      setGauge(mbps / gaugeMax);
      const text = mbps.toFixed(2);
      setReadout("Download", text, "Mbps");
      ui.download.textContent    = text;
      ui.downloadSub.textContent = fmtBytes(bytes);
    }, CFG.sampleEvery);

    // Stop after duration
    await sleep(CFG.testDuration);
    ctrl.abort();
    clearInterval(ticker);
    await Promise.allSettled(workers);

    // Final number from stable window
    const refBytes = stableStart ? bytes - stableStartBytes : bytes;
    const refStart = stableStart || start;
    const seconds  = (performance.now() - refStart) / 1000;
    const mbps     = seconds > 0 ? (refBytes * 8) / seconds / 1_000_000 : 0;
    ui.download.textContent    = mbps.toFixed(2);
    ui.downloadSub.textContent = fmtBytes(bytes);
    return mbps;
  }

  async function runUpload() {
    activeResult("upload");
    setReadout("Upload", "0.00", "Mbps");
    const ctrl = new AbortController();
    const start = performance.now();
    let bytes = 0;
    let stableStart = 0, stableStartBytes = 0;
    let gaugeMax = CFG.gaugeMaxStart;

    // Pre-build a random payload once and reuse — cheaper than rebuilding per request.
    const payload = new Uint8Array(CFG.chunkBytes);
    crypto.getRandomValues(payload.subarray(0, Math.min(65536, CFG.chunkBytes)));
    // Fill remainder by tiling the random head — incompressibility for the
    // first 64 KiB is plenty for transports that compress.
    for (let off = 65536; off < payload.length; off += 65536) {
      payload.copyWithin(off, 0, Math.min(65536, payload.length - off));
    }

    // Use XHR (not fetch) for uploads so we can read XMLHttpRequestUpload
    // progress events. fetch() resolves only when the request *completes*,
    // so with 25 MiB chunks the gauge would sit at 0 for several seconds
    // before jumping when the first chunk lands. xhr.upload.onprogress
    // fires every ~50–100 ms with the cumulative bytes sent on the wire,
    // which gives us a smooth live line at any link speed.
    const workers = [];
    for (let i = 0; i < CFG.streams; i++) {
      workers.push((async () => {
        while (performance.now() - start < CFG.testDuration && !ctrl.signal.aborted) {
          await uploadOne(payload, ctrl.signal, (delta) => { bytes += delta; });
        }
      })());
    }

    const ticker = setInterval(() => {
      const elapsedMs = performance.now() - start;
      if (stableStart === 0 && elapsedMs >= CFG.rampUpMs) {
        stableStart = performance.now();
        stableStartBytes = bytes;
      }
      const refStart = stableStart || start;
      const refBytes = stableStart ? bytes - stableStartBytes : bytes;
      const seconds  = (performance.now() - refStart) / 1000;
      const mbps = seconds > 0 ? (refBytes * 8) / seconds / 1_000_000 : 0;
      while (mbps > gaugeMax * 0.85) gaugeMax *= 2;
      setGauge(mbps / gaugeMax);
      const text = mbps.toFixed(2);
      setReadout("Upload", text, "Mbps");
      ui.upload.textContent    = text;
      ui.uploadSub.textContent = fmtBytes(bytes);
    }, CFG.sampleEvery);

    await sleep(CFG.testDuration);
    ctrl.abort();
    clearInterval(ticker);
    await Promise.allSettled(workers);

    const refBytes = stableStart ? bytes - stableStartBytes : bytes;
    const refStart = stableStart || start;
    const seconds  = (performance.now() - refStart) / 1000;
    const mbps     = seconds > 0 ? (refBytes * 8) / seconds / 1_000_000 : 0;
    ui.upload.textContent    = mbps.toFixed(2);
    ui.uploadSub.textContent = fmtBytes(bytes);
    return mbps;
  }

  function sleep(ms) { return new Promise(r => setTimeout(r, ms)); }

  // uploadOne POSTs `payload` to /upload and resolves when the request
  // finishes (success, error, or abort). It calls `onProgress(delta)` with
  // the bytes-uploaded delta as the browser sends them — typically every
  // 50–100 ms — so callers can update a live counter without waiting for
  // the whole request to finish. Honors AbortSignal by aborting the XHR.
  function uploadOne(payload, signal, onProgress) {
    return new Promise((resolve) => {
      const xhr = new XMLHttpRequest();
      let lastLoaded = 0;
      xhr.open("POST", "/upload", true);
      xhr.setRequestHeader("Content-Type", "application/octet-stream");

      xhr.upload.onprogress = (ev) => {
        if (!ev.lengthComputable) return;
        const delta = ev.loaded - lastLoaded;
        lastLoaded = ev.loaded;
        if (delta > 0) onProgress(delta);
      };

      const onAbort = () => { try { xhr.abort(); } catch (_) {} };
      const finish = () => {
        signal.removeEventListener("abort", onAbort);
        resolve();
      };
      xhr.onload = finish;
      xhr.onerror = finish;
      xhr.onabort = finish;
      xhr.ontimeout = finish;

      signal.addEventListener("abort", onAbort);
      xhr.send(payload);
    });
  }

  // How long to hold each phase's final number on the gauge before the
  // next phase starts — gives the user a beat to read the result.
  const PHASE_HOLD_MS = 1500;

  // Smoothly bring the gauge needle/arc back to 0 between phases so the
  // transition reads as "ok, on to the next thing" rather than a snap.
  async function gaugeReset() {
    setGauge(0);
    await sleep(350); // matches the CSS transition + a hair
  }

  async function runTest() {
    if (ui.go.classList.contains("running")) return;
    ui.go.classList.add("running");
    ui.gaugeWrap.classList.add("testing");
    ui.gaugeWrap.classList.remove("showing-result");
    setGauge(0);

    try {
      await runPing();
      await sleep(PHASE_HOLD_MS);   // let the user read the ping result
      await gaugeReset();

      await runDownload();
      await sleep(PHASE_HOLD_MS);   // let the user read the download result
      await gaugeReset();

      await runUpload();
      await sleep(PHASE_HOLD_MS);   // let the user read the upload result

      // Final summary frame on the big readout, then fade it out.
      activeResult(null);
      setReadout("Done", ui.download.textContent, "Mbps");
      setGauge(0);
      // Hand back to the GO button after a short "result is the headline"
      // beat — the per-phase cards below already show the full breakdown.
      await sleep(2200);
    } catch (e) {
      console.error(e);
      setReadout("Error", "—", "");
      await sleep(1500);
    } finally {
      ui.go.classList.remove("running");
      ui.gaugeWrap.classList.remove("testing");
      activeResult(null);
    }
  }

  // ----------- Boot -----------
  buildTicks();
  setGauge(0);
  fetchInfo();
  ui.go.addEventListener("click", runTest);
})();
