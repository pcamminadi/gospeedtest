// Package cli runs the speed test from a terminal with a Bubble Tea TUI.
//
// The TUI shows an animated horizontal gauge, a live Mbps readout, and
// per-phase results. Pass --json to skip the TUI and write a single JSON
// summary instead — useful for scripts or non-tty environments.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/pcamminadi/gospeedtest/internal/speedtest"
)

// Options configures one CLI run.
type Options struct {
	ServerURL string
	JSON      bool
	Duration  time.Duration
	Streams   int
}

// Run executes the test. With JSON=true it prints a JSON summary;
// otherwise it boots the Bubble Tea UI.
func Run(ctx context.Context, opts Options) error {
	if opts.ServerURL == "" {
		return errors.New("--server is required")
	}
	cfg := speedtest.Config{
		ServerURL:    opts.ServerURL,
		TestDuration: opts.Duration,
		Streams:      opts.Streams,
	}
	if opts.JSON {
		return runJSON(ctx, cfg)
	}
	return runTUI(ctx, cfg, opts.ServerURL)
}

// ---------- JSON mode ----------

func runJSON(ctx context.Context, cfg speedtest.Config) error {
	res, err := speedtest.Run(ctx, cfg, nil)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(res)
}

// ---------- TUI mode ----------

type sampleMsg speedtest.Sample
type doneMsg struct {
	res speedtest.Result
	err error
}
type infoMsg struct {
	clientIP, isp, location, serverHost string
}
type tickMsg time.Time

type model struct {
	server string

	phase      speedtest.Phase
	pingMs     float64
	jitterMs   float64
	downMbps   float64
	upMbps     float64
	currentMps float64
	bytes      int64
	displayMps float64 // smoothed for animation

	err  error
	done bool

	clientIP, isp, location, serverHost string
	localIP, gateway                    string

	width, height int
}

func newModel(server string) model {
	localIP, _ := localOutboundIP()
	gw, _ := defaultGateway()
	return model{server: server, localIP: localIP, gateway: gw}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(fetchInfoCmd(m.server), tickCmd())
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			return m, tea.Quit
		}

	case sampleMsg:
		s := speedtest.Sample(msg)
		m.phase = s.Phase
		m.currentMps = s.Mbps
		m.bytes = s.Bytes
		switch s.Phase {
		case speedtest.PhasePing:
			m.pingMs = s.PingMs
		case speedtest.PhaseDownload:
			m.downMbps = s.Mbps
		case speedtest.PhaseUpload:
			m.upMbps = s.Mbps
		}
		return m, nil

	case doneMsg:
		m.done = true
		m.err = msg.err
		if msg.err == nil {
			m.pingMs = msg.res.PingMs
			m.jitterMs = msg.res.JitterMs
			m.downMbps = msg.res.DownloadMbps
			m.upMbps = msg.res.UploadMbps
		}
		// Hold the final frame briefly so the user can read it, then quit.
		return m, tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg { return tea.Quit() })

	case infoMsg:
		m.clientIP = msg.clientIP
		m.isp = msg.isp
		m.location = msg.location
		m.serverHost = msg.serverHost
		return m, nil

	case tickMsg:
		// Smoothly catch the displayed value up to the current sample so the
		// gauge animates instead of stepping.
		const alpha = 0.30
		m.displayMps += (m.currentMps - m.displayMps) * alpha
		return m, tickCmd()
	}
	return m, nil
}

// ---------- View ----------

var (
	colAccent  = lipgloss.Color("#00d4ff")
	colAccent2 = lipgloss.Color("#7c5cff")
	colAccent3 = lipgloss.Color("#ff4d8d")
	colDim     = lipgloss.Color("#8a93a6")
	colFG      = lipgloss.Color("#e8ecf2")
)

func (m model) View() string {
	title := lipgloss.NewStyle().Bold(true).Foreground(colAccent).Render("◢ gospeedtest")
	subtitle := lipgloss.NewStyle().Foreground(colDim).Render(" — " + m.server)

	parts := []string{
		"",
		title + subtitle,
		"",
		m.renderGauge(),
		"",
		m.renderResults(),
		"",
		m.renderInfo(),
		"",
		lipgloss.NewStyle().Foreground(colDim).Render("  press q or ctrl+c to quit"),
	}
	if m.err != nil {
		parts = append(parts, "", lipgloss.NewStyle().Foreground(colAccent3).Render("  error: "+m.err.Error()))
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m model) renderGauge() string {
	const segments = 36
	max := chooseMax(m.displayMps)
	current := m.displayMps
	var label, unit string
	switch m.phase {
	case speedtest.PhasePing:
		label, unit = "PING", "ms"
		current = m.pingMs
		max = 200
	case speedtest.PhaseDownload:
		label, unit = "DOWNLOAD", "Mbps"
	case speedtest.PhaseUpload:
		label, unit = "UPLOAD", "Mbps"
	case speedtest.PhaseDone:
		label, unit = "DONE", "Mbps"
		current = m.downMbps
	default:
		label, unit = "READY", "Mbps"
	}

	frac := current / max
	if frac > 1 {
		frac = 1
	}
	if frac < 0 {
		frac = 0
	}

	filled := int(frac * float64(segments))
	var bar strings.Builder
	for i := 0; i < segments; i++ {
		if i < filled {
			c := gradientColor(float64(i) / float64(segments))
			bar.WriteString(lipgloss.NewStyle().Foreground(c).Render("█"))
		} else {
			bar.WriteString(lipgloss.NewStyle().Foreground(colDim).Render("░"))
		}
	}

	value := lipgloss.NewStyle().Foreground(colFG).Bold(true).Render(fmt.Sprintf("%7.2f", current))
	unitS := lipgloss.NewStyle().Foreground(colDim).Render(" " + unit)
	phase := lipgloss.NewStyle().Foreground(colDim).Render(fmt.Sprintf("%-9s", label))

	scaleRight := fmt.Sprintf("%g", max)
	pad := segments - 1 - len(scaleRight)
	if pad < 0 {
		pad = 0
	}
	scale := lipgloss.NewStyle().Foreground(colDim).Render("0" + strings.Repeat(" ", pad) + scaleRight)

	return lipgloss.JoinVertical(lipgloss.Left,
		"  "+phase+"  "+value+unitS,
		"  "+bar.String(),
		"  "+scale,
	)
}

func gradientColor(f float64) lipgloss.Color {
	stops := []struct{ r, g, b int }{
		{0x00, 0xd4, 0xff},
		{0x7c, 0x5c, 0xff},
		{0xff, 0x4d, 0x8d},
	}
	if f <= 0 {
		return rgbHex(stops[0])
	}
	if f >= 1 {
		return rgbHex(stops[2])
	}
	if f < 0.5 {
		return rgbHex(lerp(stops[0], stops[1], f/0.5))
	}
	return rgbHex(lerp(stops[1], stops[2], (f-0.5)/0.5))
}

func lerp(a, b struct{ r, g, b int }, t float64) struct{ r, g, b int } {
	return struct{ r, g, b int }{
		r: int(float64(a.r) + (float64(b.r)-float64(a.r))*t),
		g: int(float64(a.g) + (float64(b.g)-float64(a.g))*t),
		b: int(float64(a.b) + (float64(b.b)-float64(a.b))*t),
	}
}

func rgbHex(c struct{ r, g, b int }) lipgloss.Color {
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", c.r, c.g, c.b))
}

func chooseMax(mbps float64) float64 {
	for _, m := range []float64{50, 100, 250, 500, 1000, 2500, 5000, 10000} {
		if mbps < m*0.85 {
			return m
		}
	}
	return 10000
}

func (m model) renderResults() string {
	row := func(label, value, unit, sub string, color lipgloss.Color) string {
		head := lipgloss.NewStyle().Foreground(color).Bold(true).Width(10).Render(label)
		val := lipgloss.NewStyle().Foreground(colFG).Bold(true).Render(value)
		u := lipgloss.NewStyle().Foreground(colDim).Render(" " + unit)
		s := ""
		if sub != "" {
			s = lipgloss.NewStyle().Foreground(colDim).Render("   " + sub)
		}
		return "  " + head + val + u + s
	}
	pingSub := ""
	if m.jitterMs > 0 {
		pingSub = fmt.Sprintf("jitter %.1f ms", m.jitterMs)
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		row("PING", fmtNum(m.pingMs), "ms", pingSub, colAccent2),
		row("DOWNLOAD", fmtNum(m.downMbps), "Mbps", "", colAccent),
		row("UPLOAD", fmtNum(m.upMbps), "Mbps", "", colAccent3),
	)
}

func fmtNum(v float64) string {
	if v == 0 {
		return "    —"
	}
	return fmt.Sprintf("%5.2f", v)
}

func (m model) renderInfo() string {
	keyStyle := lipgloss.NewStyle().Foreground(colDim).Width(11)
	valStyle := lipgloss.NewStyle().Foreground(colFG)
	row := func(k, v string) string {
		if v == "" {
			v = "—"
		}
		return "  " + keyStyle.Render(strings.ToLower(k)) + valStyle.Render(v)
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.NewStyle().Foreground(colDim).Render("  ── connection ─────────────────────"),
		row("public IP", m.clientIP),
		row("ISP", m.isp),
		row("location", m.location),
		row("server", m.serverHost),
		row("local IP", m.localIP),
		row("gateway", m.gateway),
		row("os", runtime.GOOS+"/"+runtime.GOARCH),
	)
}

// ---------- commands ----------

func tickCmd() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func fetchInfoCmd(server string) tea.Cmd {
	return func() tea.Msg {
		type apiInfo struct {
			ClientIP   string `json:"client_ip"`
			ISP        string `json:"isp"`
			City       string `json:"city"`
			Region     string `json:"region"`
			Country    string `json:"country"`
			ServerHost string `json:"server_host"`
		}
		c := &http.Client{Timeout: 3 * time.Second}
		resp, err := c.Get(strings.TrimRight(server, "/") + "/api/info")
		if err != nil {
			return infoMsg{}
		}
		defer resp.Body.Close()
		var i apiInfo
		if err := json.NewDecoder(resp.Body).Decode(&i); err != nil {
			return infoMsg{}
		}
		loc := strings.Trim(strings.Join([]string{i.City, i.Region, i.Country}, ", "), ", ")
		return infoMsg{
			clientIP:   i.ClientIP,
			isp:        i.ISP,
			location:   loc,
			serverHost: i.ServerHost,
		}
	}
}

// ---------- entry ----------

func runTUI(ctx context.Context, cfg speedtest.Config, server string) error {
	m := newModel(server)
	p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithAltScreen())

	// Run the speedtest in a goroutine, funneling samples back to the program.
	go func() {
		res, err := speedtest.Run(ctx, cfg, func(s speedtest.Sample) {
			p.Send(sampleMsg(s))
		})
		p.Send(doneMsg{res: res, err: err})
	}()

	_, err := p.Run()
	return err
}

// ---------- network helpers ----------

// localOutboundIP returns the local IP that would be used to reach the
// internet. It does not actually open a connection — UDP "dial" is just a
// syscall that consults the route table.
func localOutboundIP() (string, error) {
	conn, err := net.Dial("udp", "8.8.8.8:53")
	if err != nil {
		return "", err
	}
	defer conn.Close()
	if a, ok := conn.LocalAddr().(*net.UDPAddr); ok {
		return a.IP.String(), nil
	}
	return "", errors.New("no local addr")
}
