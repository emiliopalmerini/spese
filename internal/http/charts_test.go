package http

import (
	"strings"
	"testing"
)

func TestRenderBarChartZeroes(t *testing.T) {
	out := string(RenderBarChart(BarChartOpts{
		Data: []int64{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		W:    240, H: 60,
	}))
	if !strings.HasPrefix(out, "<svg") {
		t.Fatalf("expected SVG prefix, got %q", out)
	}
	if got := strings.Count(out, "<rect"); got != 12 {
		t.Errorf("expected 12 rects, got %d", got)
	}
	if !strings.Contains(out, `height="0.0"`) {
		t.Errorf("zero data should produce zero-height bars")
	}
}

func TestRenderBarChartHighlight(t *testing.T) {
	out := string(RenderBarChart(BarChartOpts{
		Data: []int64{100, 200, 300},
		W:    90, H: 40,
		HighlightIdx:   1,
		Color:          "#1a1612",
		HighlightColor: "#b8451c",
	}))
	if !strings.Contains(out, `fill="#b8451c"`) {
		t.Errorf("highlight color missing")
	}
	if !strings.Contains(out, `opacity="1.00"`) {
		t.Errorf("highlight bar should be fully opaque")
	}
}

func TestRenderBarChartEmpty(t *testing.T) {
	out := RenderBarChart(BarChartOpts{Data: nil})
	if out != "" {
		t.Errorf("empty data should produce empty output, got %q", out)
	}
}

func TestRenderSparklineConstant(t *testing.T) {
	out := string(RenderSparkline(SparkOpts{
		Data: []int64{50, 50, 50, 50, 50},
		W:    50, H: 10,
	}))
	if !strings.Contains(out, "<polyline") {
		t.Errorf("expected polyline element")
	}
}

func TestRenderSparklineTooFewPoints(t *testing.T) {
	if RenderSparkline(SparkOpts{Data: []int64{42}}) != "" {
		t.Errorf("single-point sparkline should be empty")
	}
	if RenderSparkline(SparkOpts{Data: nil}) != "" {
		t.Errorf("nil sparkline should be empty")
	}
}

func TestRenderDonutTwoEqualSlices(t *testing.T) {
	out := string(RenderDonut(DonutOpts{
		Data: []DonutSlice{
			{Amount: 100, Color: "#aaa"},
			{Amount: 100, Color: "#bbb"},
		},
		Size: 120, Thickness: 18,
	}))
	if strings.Count(out, "<circle") != 3 {
		t.Errorf("expected 3 circles (bg + 2 slices), got %d", strings.Count(out, "<circle"))
	}
	if !strings.Contains(out, `stroke="#aaa"`) || !strings.Contains(out, `stroke="#bbb"`) {
		t.Errorf("slice colors missing")
	}
}

func TestRenderDonutEmpty(t *testing.T) {
	out := string(RenderDonut(DonutOpts{Data: nil}))
	if !strings.Contains(out, "<circle") {
		t.Errorf("empty donut should still render bg ring")
	}
	if strings.Count(out, "<circle") != 1 {
		t.Errorf("empty donut should have only 1 circle (bg)")
	}
}

func TestPaletteColorStable(t *testing.T) {
	a := PaletteColor("Casa")
	b := PaletteColor("Casa")
	if a != b {
		t.Errorf("PaletteColor not deterministic: %q vs %q", a, b)
	}
}

func TestPaletteColorEmpty(t *testing.T) {
	if PaletteColor("") != QuadernoPalette[0] {
		t.Errorf("empty key should return first palette slot")
	}
}

func TestPaletteColorInRamp(t *testing.T) {
	got := PaletteColor("Cibo")
	in := false
	for _, c := range QuadernoPalette {
		if c == got {
			in = true
			break
		}
	}
	if !in {
		t.Errorf("PaletteColor returned %q, not in ramp", got)
	}
}
