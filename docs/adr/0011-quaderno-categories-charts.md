# ADR-0011: Quaderno categories panel, donut, monthly bar chart

## Status
Proposed

## Context
ADR-0008 (tokens) + ADR-0009 (dashboard chrome) leave the **Per categoria**
section (`partials/category_breakdown.html` + `partials/month_categories.html`)
and the implicit "monthly trend" view rendered as plain rows with no chart.

Quaderno (`v1-quaderno.jsx → QDashboard` + `QCategories`) shows:

1. **Andamento mensile** (Section I): inline SVG bar chart, 12 monthly
   totals, current month highlighted in terracotta, others in ink at 55%
   opacity, future months at 25%. Min / Media / Max footer in mono uppercase.
2. **Per categoria** (Section II): inline SVG donut on the left (`size 120`,
   `thickness 16`), top-4 categories on the right with color dot + name + `%`
   (mono) + serif amount, "Tutte →" link to full panel.
3. **Categorie** full screen (deep-link from "Tutte"): bigger donut
   (`size 180`, `thickness 26`) with **Totale** centered inside, full list
   below as bars with `txCount` + percent.

This ADR ports the three views. Charts are **server-rendered SVG** (no JS
charts library), keeping with the existing `text/template` pipeline.

## Decision

### Chart helpers (`internal/http/charts.go`, new)
Pure-Go inline-SVG renderers; each returns `template.HTML`:

```go
type BarChartOpts struct {
    Data           []int64
    W, H           int
    Color          string
    HighlightIdx   int
    HighlightColor string
    Labels         []string // 12 IT month abbrev
    LabelColor     string
}
func RenderBarChart(o BarChartOpts) template.HTML

type DonutSlice struct{ Amount int64; Color string }
type DonutOpts struct{ Data []DonutSlice; Size, Thickness int; Bg string }
func RenderDonut(o DonutOpts) template.HTML

type SparkOpts struct{ Data []int64; W, H int; Stroke string }
func RenderSparkline(o SparkOpts) template.HTML
```

Implementations mirror the SVG primitives in `shared.jsx` (`BarChart`,
`Donut`, `Sparkline`) line-for-line — same coordinates, same rounding.
Pure functions; testable.

Wire into the template `FuncMap`:
```go
"barChart":   func(o BarChartOpts) template.HTML { return RenderBarChart(o) },
"donut":      func(o DonutOpts)    template.HTML { return RenderDonut(o) },
"sparkline":  func(o SparkOpts)    template.HTML { return RenderSparkline(o) },
```

### View-models
- `MonthlyTrendVM`: `{Series [12]int64, Labels [12]string, HighlightIdx int, Min, Mean, Max int64}`.
- `CategoryBreakdownVM` (existing one extended): each row gets a deterministic
  `Color` derived from the **palette table** below; `Pct` is rounded int.
- `CategoriesPageVM`: `{Total int64, Rows []CategoryBreakdownRow, Period string}`.

### Palette
Greyscale ramp (Quaderno uses ink-only fills for the donut, with the slice
position implying the order):
```
['#3a3a3a','#5a5a5a','#7a7a7a','#9a9a9a','#b3b3b3','#c8c8c8','#dcdcdc','#a83c2e']
```
Last slot reserved for "over budget" / negative slices (red). Palette lives
in `internal/http/palette.go` keyed by category name with a stable hash
fallback for unknown categories so colors don't reshuffle when categories
are added.

### Endpoints
- `GET /ui/dashboard/monthly-trend` (new) → `partials/monthly_trend.html`.
- `GET /ui/dashboard/categories?period=...` (existing): re-render with new
  `partials/category_breakdown.html`.
- `GET /ui/categories?period=...` (new full page) → `pages/categories.html`,
  reachable via "Tutte →" link from the dashboard panel and from a future
  tab-bar entry. For this ADR, the link target is registered but the route
  may remain feature-flagged behind `ENABLE_CATEGORIES_PAGE=true`.

### Templates
`partials/monthly_trend.html`:
```html
{{template "section_label" dict "Num" "I" "Title" "Andamento mensile" "Sub" "Ultimi 12 mesi"}}
<div class="chart-wrap">
  {{barChart (dict "Data" .Series "W" 336 "H" 92 "Color" "#1a1612"
              "HighlightIdx" .HighlightIdx "HighlightColor" "#b8451c"
              "Labels" .Labels "LabelColor" "#8a847a")}}
</div>
<div class="chart-stats label-mono">
  <span>Min {{.MinFmt}} €</span>
  <span>Media {{.MeanFmt}} €</span>
  <span>Max {{.MaxFmt}} €</span>
</div>
```

`partials/category_breakdown.html` (rewrite):
```html
{{template "section_label" dict "Num" "II" "Title" "Per categoria"
   "Sub" "Mese in corso"
   "Action" "<a class=\"chip\" href=\"/ui/categories\">Tutte →</a>"}}
<div class="cat-panel">
  <div class="cat-panel__chart">
    {{donut (dict "Data" .DonutData "Size" 120 "Thickness" 16 "Bg" "rgba(26,22,18,0.06)")}}
  </div>
  <div class="cat-panel__rows">
    {{range .TopRows}}
      <div class="cat-row">
        <span class="cat-row__dot" style="background:{{.Color}}"></span>
        <span class="cat-row__name">{{.Name}}</span>
        <span class="cat-row__pct label-mono">{{.Pct}}%</span>
        <span class="cat-row__amt">{{.AmtFmt}}</span>
      </div>
    {{end}}
  </div>
</div>
```

`pages/categories.html`: full page with header, period chips (existing
`.chip` from ADR-0008), centered `size 180` donut with overlaid Totale,
then the full row list with thin progress bars (`height 2px`, ink fill at
slice color, `rgba(26,22,18,0.08)` track).

### CSS (`web/static/css/dashboard.css` additions)
```css
.chart-wrap{margin-top:18px;color:var(--ink);}
.chart-stats{display:flex;justify-content:space-between;margin-top:8px;}
.cat-panel{display:flex;gap:18px;align-items:center;margin-top:16px;}
.cat-panel__rows{flex:1;display:flex;flex-direction:column;gap:8px;}
.cat-row{display:grid;grid-template-columns:auto 1fr auto auto;align-items:baseline;gap:8px;}
.cat-row__dot{width:8px;height:8px;border-radius:2px;display:inline-block;}
.cat-row__name{font-size:13px;color:var(--ink);overflow:hidden;text-overflow:ellipsis;white-space:nowrap;}
.cat-row__pct{font-size:10px;}
.cat-row__amt{font-family:var(--font-display);font-size:16px;}
```

`pages/categories.html`-specific styles in `categories.css` (new file): donut
overlay, full-row layout with progress bar.

## Inputs / Outputs

| Op | Input | Output |
|---|---|---|
| Monthly trend | year | `[12]int64 series + min/mean/max` rendered as SVG bar chart |
| Category panel (top 4) | period | donut SVG + 4 rows |
| Categories full page | period | donut + N rows with progress bars + tx count |

## Edge cases
- Year with zero spend → bar chart renders 12 zero-height bars (no division
  by zero); footer shows `Min 0 € / Media 0 € / Max 0 €`.
- Single category present → donut is a full circle of one color; first row
  is `100%`.
- New / unknown category → falls back to deterministic hash slot in palette,
  not the red "over budget" slot.
- `period=year` with `HighlightIdx` ambiguous → no highlight (–1).
- SVG accessibility: each chart has `role="img"` + `<title>` describing
  total spend.

## Error conditions
- DB error → 500 + slog (existing pattern).
- Invalid `period` → 400 (existing).

## Test plan
1. **Acceptance** (`internal/http/server_test.go`):
   - GET `/ui/dashboard/monthly-trend?year=2026` returns markup with 12
     `<rect>` elements and a fill matching `#b8451c` on the highlight bar.
   - GET `/ui/dashboard/categories?period=month` returns a `<svg>` for the
     donut + at least one `.cat-row`.
2. **Unit** (`internal/http/charts_test.go`):
   - `RenderBarChart` with all-zero data → 12 rects, all `height="0"`.
   - `RenderDonut` with two equal slices → each `stroke-dasharray` is `circ/2`.
   - `RenderSparkline` with constant data → flat line at midline.
   - Palette hash: same name → same color across calls.
3. **Integration**:
   - End-to-end with seeded SQLite: monthly totals + category sums match
     between handler responses and direct repository queries.

## Out of scope
- Treemap (used by Banca / Spaziale variations, not Quaderno).
- Calendar heatmap (Fluido variation).
- Per-category drill-down screen (touched by ADR-0014).
- Chart interactivity (hover, click): static SVG only in v1.
