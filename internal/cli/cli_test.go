package cli

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestChooseMax(t *testing.T) {
	cases := []struct {
		mbps float64
		want float64
	}{
		{0, 50},
		{10, 50},
		{42, 50},
		// Boundary: 50*0.85 = 42.5, so 42.5 is NOT < 42.5 → falls to next bucket.
		{42.5, 100},
		{50, 100},
		{84, 100},
		{200, 250},
		{800, 1000},
		{2000, 2500},
		{100_000, 10_000},
	}
	for _, tc := range cases {
		got := chooseMax(tc.mbps)
		if got != tc.want {
			t.Errorf("chooseMax(%v) = %v, want %v", tc.mbps, got, tc.want)
		}
	}
}

func TestRgbHex(t *testing.T) {
	cases := []struct {
		c    struct{ r, g, b int }
		want lipgloss.Color
	}{
		{struct{ r, g, b int }{0, 0, 0}, lipgloss.Color("#000000")},
		{struct{ r, g, b int }{255, 255, 255}, lipgloss.Color("#ffffff")},
		{struct{ r, g, b int }{0x00, 0xd4, 0xff}, lipgloss.Color("#00d4ff")},
	}
	for _, tc := range cases {
		got := rgbHex(tc.c)
		if got != tc.want {
			t.Errorf("rgbHex(%+v) = %q, want %q", tc.c, got, tc.want)
		}
	}
}

func TestLerp(t *testing.T) {
	a := struct{ r, g, b int }{0, 0, 0}
	b := struct{ r, g, b int }{100, 200, 50}

	// t=0 -> a, t=1 -> b, t=0.5 -> midpoint
	if got := lerp(a, b, 0); got != a {
		t.Errorf("lerp t=0 = %+v, want %+v", got, a)
	}
	if got := lerp(a, b, 1); got != b {
		t.Errorf("lerp t=1 = %+v, want %+v", got, b)
	}
	mid := lerp(a, b, 0.5)
	if mid.r != 50 || mid.g != 100 || mid.b != 25 {
		t.Errorf("lerp t=0.5 = %+v, want {50 100 25}", mid)
	}
}

func TestGradientColorEndpoints(t *testing.T) {
	// Ensures gradient terminates at the documented stops; midpoints are
	// covered indirectly via lerp.
	if got := gradientColor(0); got != lipgloss.Color("#00d4ff") {
		t.Errorf("gradient(0) = %q, want #00d4ff", got)
	}
	if got := gradientColor(1); got != lipgloss.Color("#ff4d8d") {
		t.Errorf("gradient(1) = %q, want #ff4d8d", got)
	}
	if got := gradientColor(0.5); got != lipgloss.Color("#7c5cff") {
		t.Errorf("gradient(0.5) = %q, want #7c5cff", got)
	}
}

func TestFmtNum(t *testing.T) {
	if got := fmtNum(0); got != "    —" {
		t.Errorf("fmtNum(0) = %q, want %q", got, "    —")
	}
	if got := fmtNum(12.34); got != "12.34" {
		t.Errorf("fmtNum(12.34) = %q, want %q", got, "12.34")
	}
}
