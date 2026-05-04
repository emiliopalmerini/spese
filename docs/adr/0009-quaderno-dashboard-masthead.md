# ADR-0009: Quaderno dashboard masthead, hero number, KPI strip

## Status
Accepted (partial — KPI strip endpoint deferred; existing stat_pills + stat_grid kept and restyled by ADR-0008 tokens. Bar chart in ADR-0011.)

## Context
ADR-0008 ports tokens + chrome. The dashboard (`pages/dashboard.html` +
`partials/stat_hero.html` + `partials/stat_pills.html` + `partials/stat_grid.html`)
still renders as the previous monochrome layout. Quaderno (variation 01,
`v1-quaderno.jsx → QDashboard`) reframes it as a magazine page:

- **Masthead**: small mono "Anno · 2026  ·  Edizione N" line, big serif month
  name (Italian, e.g. "Maggio"), mono "Diario delle spese" sub-line.
- **Hero number**: serif `72px` integer + `28px` decimals + `€` glyph; below,
  a mono `↑/↓ N% vs. mese precedente · X €/giorno` line, color-coded
  (positive = green, negative = red).
- **KPI strip**: 2-column dotted-divider strip with **Entrate** and **Risparmio %**
  + 70×16 sparklines, replacing `stat-pills` + `stat-grid`.
- **Section labels**: roman numeral + accent italic + serif title + mono sub
  ("II. Per categoria · MESE IN CORSO"), replacing `<h3 class="section-title">`.

This ADR ports those four pieces. Charts, tables, accordions, recurrents,
transactions list keep current markup; only the heading + hero + KPI region
changes.

## Decision

### Service / handler changes
- `internal/services/dashboard_service.go` (or wherever stat hero is computed):
  expose three extra fields on the hero view-model:
  - `MonthLabelLong` — Italian long month name from `MONTHS_IT_LONG`.
  - `Year` — current year as int.
  - `EditionNum` — month index + 1 (Roman numeral rendered in template).
  - `DeltaPctVsPrev` — signed percent vs. previous month spend (rounded int).
  - `DailyRunRate` — current month spend / day-of-month (cents).
- KPI strip endpoint `/ui/dashboard/kpi-strip` (new):
  - Returns rendered `partials/kpi_strip.html` with two cards:
    - **Entrate**: current-month income + 12-month income sparkline.
    - **Risparmio**: `(income - spend)/income * 100` rounded int + 12-month
      net sparkline.
  - View-model: `{IncomeCents int64, SavingsPct int, IncomeSeries [12]int64, NetSeries [12]int64}`.
- Old `/ui/dashboard/stat-pills` + `/ui/dashboard/stat-grid` endpoints kept for
  backward-compatibility one ADR cycle, then deleted in ADR-0010 follow-up.

### Templates

`partials/stat_hero.html` (rewrite):
```html
<div class="masthead">
  <div class="masthead__meta">
    <span>Anno · {{.Year}}</span>
    <span>Edizione №{{.EditionNum}}</span>
  </div>
  <h1 class="masthead__month">{{.MonthLabelLong}}</h1>
  <div class="label-mono">Diario delle spese</div>
</div>
<div class="hero-number">
  <div class="label-mono">Speso questo mese</div>
  <div class="hero-number__value">
    <span class="hero-number__int">{{.SpentInt}}</span>
    <span class="hero-number__dec">,{{.SpentDec}}</span>
    <span class="hero-number__cur">€</span>
  </div>
  <div class="hero-number__delta">
    <span class="{{if gt .DeltaPctVsPrev 0}}is-negative{{else}}is-positive{{end}}">
      {{if gt .DeltaPctVsPrev 0}}↑{{else}}↓{{end}} {{abs .DeltaPctVsPrev}}% vs. {{.PrevMonthShort}}
    </span>
    <span>·</span>
    <span>{{.DailyRunRateFmt}} € / giorno</span>
  </div>
</div>
```

`partials/kpi_strip.html` (new): two `.kpi` cells separated by a left dotted
border on the right cell; each cell = mono label + serif value + sparkline SVG.

`partials/section_label.html` (new): renders
```html
<div class="section-label">
  <span class="section-label__num">{{.Num}}.</span>
  <div>
    <div class="section-label__title">{{.Title}}</div>
    <div class="section-label__sub label-mono">{{.Sub}}</div>
  </div>
  {{if .Action}}<div class="section-label__action">{{.Action | safe}}</div>{{end}}
</div>
```

`pages/dashboard.html`: replace existing `stat-pills` + `stat-grid` blocks
with a single `<div id="kpi-strip" hx-get="/ui/dashboard/kpi-strip">`.
Replace each `<h3 class="section-title">` with `{{template "section_label" .}}`.

### CSS (`web/static/css/dashboard.css`)
```css
.masthead{padding:64px 0 18px;}
.masthead__meta{
  display:flex;justify-content:space-between;align-items:baseline;
  font-family:var(--font-mono);font-size:var(--text-xs);
  letter-spacing:0.16em;color:var(--muted);text-transform:uppercase;
}
.masthead__month{
  font-family:var(--font-display);font-weight:400;
  font-size:clamp(2.5rem,1.5rem+4vw,3.5rem);
  line-height:1;margin:14px 0 4px;letter-spacing:-0.01em;
}
.hero-number{padding:32px 0 24px;border-top:1px dotted var(--line);}
.hero-number__value{display:flex;align-items:baseline;gap:8px;}
.hero-number__int{font-family:var(--font-display);font-size:72px;line-height:0.95;letter-spacing:-0.03em;}
.hero-number__dec,.hero-number__cur{font-family:var(--font-display);font-size:28px;color:var(--ink-2);}
.hero-number__delta{margin-top:10px;display:flex;gap:14px;align-items:center;}
.hero-number__delta .is-negative{color:var(--negative);}
.hero-number__delta .is-positive{color:var(--positive);}
.kpi-strip{display:grid;grid-template-columns:1fr 1fr;border-top:1px dotted var(--line);border-bottom:1px dotted var(--line);}
.kpi-strip__cell{padding:16px 20px 18px;}
.kpi-strip__cell + .kpi-strip__cell{border-left:1px dotted var(--line);text-align:right;}
.kpi-strip__value{font-family:var(--font-display);font-size:32px;line-height:1;letter-spacing:-0.02em;}
.kpi-strip__suffix{font-family:var(--font-display);font-size:14px;color:var(--ink-2);margin-left:4px;}
.section-label{display:flex;align-items:baseline;gap:12px;margin-bottom:var(--space-4);}
.section-label__num{font-family:var(--font-display);font-style:italic;color:var(--accent);font-size:14px;}
.section-label__title{font-family:var(--font-display);font-size:22px;line-height:1.1;letter-spacing:-0.01em;}
.section-label__sub{margin-top:2px;}
```

## Inputs / Outputs

| Op | Input | Output |
|---|---|---|
| Hero VM | year, month, prev-month total | masthead + hero VM with delta + run-rate |
| KPI strip | year | income current + sparkline + savings pct + net sparkline |
| Section label | num (Roman), title, sub | rendered HTML |

## Edge cases
- First month of year (`EditionNum == 1`): `PrevMonthShort` from December of
  previous year; `DeltaPctVsPrev` uses `MONTHLY_SPESE` from prior year if
  available, else `0` and arrow hidden.
- Zero income → `SavingsPct = 0`; sparkline still rendered (flat).
- `dayOfMonth == 0` impossible; `DailyRunRate` divisor minimum `1`.
- Sparkline of all-equal values → flat horizontal line at midline.

## Error conditions
- DB error → 500 + slog (existing pattern).
- Invalid year query string → 400.

## Test plan
1. **Acceptance** (`internal/http/server_test.go`):
   - Seed expenses for current + previous month → GET `/ui/dashboard/stat-hero`
     includes `<h1 class="masthead__month">{{currentItalianMonth}}</h1>` and a
     `↑` or `↓` delta with the correct sign.
   - Seed incomes + expenses → GET `/ui/dashboard/kpi-strip` returns both
     `Entrate` and `Risparmio` cells with non-empty values.
2. **Unit**:
   - Roman numeral helper for 1..12.
   - Italian-month helper for 0..11.
   - Delta-pct helper: prev=0 → 0; prev>0 → rounded percent; sign preserved.
3. **Integration**:
   - Render `dashboard_page` template; ensure no template parse errors and
     the section labels are present for **Per categoria**, **Spese ricorrenti**,
     **Pick Months**, **Cash Flow**, **Proiezioni**, **Entrate per Categoria**,
     **Ultime transazioni**.

## Out of scope
- Bar chart "andamento mensile" port (deferred to ADR-0011 charts pass).
- Treemap, calendar heatmap (other variations; not Quaderno).
- Recurrents / categories / transactions list restyle (their own ADRs).
