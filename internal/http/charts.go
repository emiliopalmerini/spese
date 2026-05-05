package http

import (
	"fmt"
	"hash/fnv"
	"html/template"
	"math"
	"strings"
)

// BarChartOpts configures RenderBarChart.
type BarChartOpts struct {
	Data           []int64
	W, H           int
	Color          string
	HighlightIdx   int
	HighlightColor string
	Labels         []string
	LabelColor     string
	Gap            int
	ShowLabels     bool
}

// RenderBarChart returns an inline SVG bar chart for the given series.
// Mirrors the JSX prototype's BarChart in shared.jsx so that visual output
// matches the design pixel-for-pixel.
func RenderBarChart(o BarChartOpts) template.HTML {
	if o.W == 0 {
		o.W = 320
	}
	if o.H == 0 {
		o.H = 80
	}
	if o.Gap == 0 {
		o.Gap = 4
	}
	if o.Color == "" {
		o.Color = "#1a1612"
	}
	if o.HighlightColor == "" {
		o.HighlightColor = "#b8451c"
	}
	if o.LabelColor == "" {
		o.LabelColor = "#8a847a"
	}

	n := len(o.Data)
	if n == 0 {
		return ""
	}
	var max int64
	for _, v := range o.Data {
		if v > max {
			max = v
		}
	}
	labelH := 0
	if o.ShowLabels {
		labelH = 14
	}
	barW := float64(o.W-o.Gap*(n-1)) / float64(n)

	var b strings.Builder
	fmt.Fprintf(&b, `<svg role="img" width="%d" height="%d" viewBox="0 0 %d %d" style="display:block">`,
		o.W, o.H+labelH, o.W, o.H+labelH)

	for i, v := range o.Data {
		var bh float64
		if max > 0 {
			bh = float64(v) / float64(max) * float64(o.H)
		}
		x := float64(i) * (barW + float64(o.Gap))
		y := float64(o.H) - bh
		c := o.Color
		opacity := 0.55
		if i == o.HighlightIdx {
			c = o.HighlightColor
			opacity = 1.0
		} else if o.HighlightIdx >= 0 && i > o.HighlightIdx {
			opacity = 0.25
		}
		fmt.Fprintf(&b,
			`<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s" opacity="%.2f" rx="1" />`,
			x, y, barW, bh, c, opacity)

		if o.ShowLabels && i < len(o.Labels) {
			fmt.Fprintf(&b,
				`<text x="%.1f" y="%d" font-size="9" fill="%s" text-anchor="middle" style="font-family:inherit;letter-spacing:0.04em">%s</text>`,
				x+barW/2, o.H+11, o.LabelColor, strings.ToUpper(o.Labels[i]))
		}
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

// SparkOpts configures RenderSparkline.
type SparkOpts struct {
	Data        []int64
	W, H        int
	Stroke      string
	StrokeWidth float64
}

// RenderSparkline returns an inline SVG sparkline polyline.
func RenderSparkline(o SparkOpts) template.HTML {
	if o.W == 0 {
		o.W = 60
	}
	if o.H == 0 {
		o.H = 18
	}
	if o.Stroke == "" {
		o.Stroke = "currentColor"
	}
	if o.StrokeWidth == 0 {
		o.StrokeWidth = 1.25
	}

	n := len(o.Data)
	if n < 2 {
		return ""
	}
	var max, min int64 = math.MinInt64, math.MaxInt64
	for _, v := range o.Data {
		if v > max {
			max = v
		}
		if v < min {
			min = v
		}
	}
	rng := float64(max - min)
	if rng == 0 {
		rng = 1
	}

	pts := make([]string, n)
	for i, v := range o.Data {
		x := float64(i) / float64(n-1) * float64(o.W)
		y := float64(o.H) - (float64(v-min)/rng)*float64(o.H-2) - 1
		pts[i] = fmt.Sprintf("%.1f,%.1f", x, y)
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<svg role="img" width="%d" height="%d" viewBox="0 0 %d %d" style="display:block">`,
		o.W, o.H, o.W, o.H)
	fmt.Fprintf(&b,
		`<polyline points="%s" fill="none" stroke="%s" stroke-width="%.2f" stroke-linejoin="round" stroke-linecap="round" />`,
		strings.Join(pts, " "), o.Stroke, o.StrokeWidth)
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

// DonutSlice represents a single arc of a donut chart.
type DonutSlice struct {
	Amount int64
	Color  string
}

// DonutOpts configures RenderDonut.
type DonutOpts struct {
	Data      []DonutSlice
	Size      int
	Thickness int
	Bg        string
	Gap       float64
}

// RenderDonut returns an inline SVG donut chart.
func RenderDonut(o DonutOpts) template.HTML {
	if o.Size == 0 {
		o.Size = 120
	}
	if o.Thickness == 0 {
		o.Thickness = 18
	}
	if o.Bg == "" {
		o.Bg = "rgba(26,22,18,0.06)"
	}
	if o.Gap == 0 {
		o.Gap = 0.012
	}
	var total int64
	for _, s := range o.Data {
		total += s.Amount
	}

	r := float64(o.Size)/2 - float64(o.Thickness)/2
	cx := float64(o.Size) / 2
	cy := float64(o.Size) / 2
	circ := 2 * math.Pi * r

	var b strings.Builder
	fmt.Fprintf(&b, `<svg role="img" width="%d" height="%d" viewBox="0 0 %d %d" style="display:block">`,
		o.Size, o.Size, o.Size, o.Size)
	fmt.Fprintf(&b,
		`<circle cx="%.2f" cy="%.2f" r="%.2f" fill="none" stroke="%s" stroke-width="%d" />`,
		cx, cy, r, o.Bg, o.Thickness)

	if total > 0 {
		var acc float64
		for _, s := range o.Data {
			frac := float64(s.Amount) / float64(total)
			length := math.Max(0, frac-o.Gap) * circ
			offset := -acc * circ
			acc += frac
			fmt.Fprintf(&b,
				`<circle cx="%.2f" cy="%.2f" r="%.2f" fill="none" stroke="%s" stroke-width="%d" stroke-dasharray="%.2f %.2f" stroke-dashoffset="%.2f" transform="rotate(-90 %.2f %.2f)" />`,
				cx, cy, r, s.Color, o.Thickness, length, circ-length, offset, cx, cy)
		}
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

// QuadernoPalette is the slice color ramp used by donut + category rows.
// Matches the JSX prototype shared.jsx CATEGORY_BREAKDOWN colors with the
// terracotta accent reserved for the over-budget / negative slot.
var QuadernoPalette = []string{
	"#b8451c", // terracotta
	"#c8954a", // ochre
	"#6b7a3d", // olive
	"#4a6b85", // slate blue
	"#8c4d6e", // plum
	"#88a37a", // sage
	"#c98b1f", // mustard
}

// PaletteColor picks a stable color for the given key, hashing into
// QuadernoPalette so the same category name always gets the same color.
func PaletteColor(key string) string {
	if key == "" {
		return QuadernoPalette[0]
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return QuadernoPalette[int(h.Sum32())%len(QuadernoPalette)]
}

// ── Template-friendly map → opts adapters ────────────────────────

func mapInt(m map[string]interface{}, k string) int {
	if v, ok := m[k]; ok {
		switch x := v.(type) {
		case int:
			return x
		case int64:
			return int(x)
		}
	}
	return 0
}
func mapFloat(m map[string]interface{}, k string) float64 {
	if v, ok := m[k]; ok {
		switch x := v.(type) {
		case float64:
			return x
		case int:
			return float64(x)
		}
	}
	return 0
}
func mapStr(m map[string]interface{}, k string) string {
	if v, ok := m[k]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
func mapBool(m map[string]interface{}, k string) bool {
	if v, ok := m[k]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}
func mapInts(m map[string]interface{}, k string) []int64 {
	v, ok := m[k]
	if !ok {
		return nil
	}
	switch x := v.(type) {
	case []int64:
		return x
	case [12]int64:
		return x[:]
	}
	return nil
}
func mapStrings(m map[string]interface{}, k string) []string {
	v, ok := m[k]
	if !ok {
		return nil
	}
	if s, ok := v.([]string); ok {
		return s
	}
	return nil
}
func mapSlices(m map[string]interface{}, k string) []DonutSlice {
	v, ok := m[k]
	if !ok {
		return nil
	}
	if s, ok := v.([]DonutSlice); ok {
		return s
	}
	return nil
}

func barChartOptsFromMap(m map[string]interface{}) BarChartOpts {
	return BarChartOpts{
		Data:           mapInts(m, "Data"),
		W:              mapInt(m, "W"),
		H:              mapInt(m, "H"),
		Color:          mapStr(m, "Color"),
		HighlightIdx:   mapInt(m, "HighlightIdx"),
		HighlightColor: mapStr(m, "HighlightColor"),
		Labels:         mapStrings(m, "Labels"),
		LabelColor:     mapStr(m, "LabelColor"),
		Gap:            mapInt(m, "Gap"),
		ShowLabels:     mapBool(m, "ShowLabels"),
	}
}
func sparkOptsFromMap(m map[string]interface{}) SparkOpts {
	return SparkOpts{
		Data:        mapInts(m, "Data"),
		W:           mapInt(m, "W"),
		H:           mapInt(m, "H"),
		Stroke:      mapStr(m, "Stroke"),
		StrokeWidth: mapFloat(m, "StrokeWidth"),
	}
}
func donutOptsFromMap(m map[string]interface{}) DonutOpts {
	return DonutOpts{
		Data:      mapSlices(m, "Data"),
		Size:      mapInt(m, "Size"),
		Thickness: mapInt(m, "Thickness"),
		Bg:        mapStr(m, "Bg"),
		Gap:       mapFloat(m, "Gap"),
	}
}
