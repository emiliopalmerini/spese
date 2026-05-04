# ADR-0012: Quaderno restyle of recurrents list and panel

## Status
Accepted (CSS + recurrent page chrome restyled; day-card avatar + monthly-grouped header VM deferred — would require new handler fields.)

## Context
ADR-0008 (tokens) + ADR-0010 (forms) leave the recurrents surfaces
(`pages/recurrent.html`, `partials/recurrent_list.html`,
`partials/recurrent-list.html`, `partials/recurrent_monthly_overview.html`)
styled with the previous brutalist row pattern: hard-bordered list cells,
sans-serif headings, no day-card.

Quaderno (`v1-quaderno.jsx → QRecurring`) shows:

- **Header**: `Capitolo IV` mono kicker + serif `Spese ricorrenti` title.
- **Total**: serif `44px` integer + serif `18px` `€ / mese` suffix; mono
  caption `Da N abbonamenti attivi`.
- **Row**: small square day-card `44 × 44` (paper-raised bg, 1px dotted
  border, serif day number, e.g. `01`), serif name (`18px`), mono sub
  (`category · freq · prox. NN MMM`), serif amount on the right.
- **CTA**: full-width dashed-border secondary button "+ Nuova ricorrente".

This ADR ports those four pieces. The page route, handlers, queries are
unchanged; only the rendered shape.

## Decision

### View-models
`RecurrentListVM`:
- `MonthlyTotalCents int64`
- `MonthlyTotalFmt string` (Italian thousands-grouped int)
- `MonthlyCount int`
- `Rows []RecurrentRowVM` with:
  - `Day int` (1..31), `DayPadded string` (`"01"`)
  - `Name string`, `CategoryName string`
  - `FreqLabel string` ("Mensile" | "Annuale" | "Settimanale")
  - `NextRunShort string` ("01 Dic")
  - `AmountFmt string` (`"720"` for whole-euro, else `"29,90"`)

### Templates

`pages/recurrent.html` — header block:
```html
<div class="page-header">
  <div class="label-mono">Capitolo IV</div>
  <h1 class="page-header__title">Spese ricorrenti</h1>
  <div class="recurrent-total">
    <span class="recurrent-total__int">{{.MonthlyTotalFmt}}</span>
    <span class="recurrent-total__suffix">€ / mese</span>
  </div>
  <div class="label-mono">Da {{.MonthlyCount}} abbonamenti attivi</div>
</div>
```

`partials/recurrent_list.html` (rewrite — pick this filename and
remove the older `recurrent-list.html` once green):
```html
<div class="recurrent-list">
  {{range $i, $r := .Rows}}
    <div class="recurrent-row">
      <div class="recurrent-row__day">{{$r.DayPadded}}</div>
      <div class="recurrent-row__main">
        <div class="recurrent-row__name">{{$r.Name}}</div>
        <div class="label-mono recurrent-row__sub">
          {{$r.CategoryName}} · {{$r.FreqLabel}} · prox. {{$r.NextRunShort}}
        </div>
      </div>
      <div class="recurrent-row__amt">{{$r.AmountFmt}} €</div>
      <div class="recurrent-row__actions">
        <button class="action-icon" hx-get="/recurrent/{{$r.ID}}/edit"
                hx-target="#sheetContent" hx-swap="innerHTML"
                aria-label="Modifica">…</button>
      </div>
    </div>
  {{end}}
</div>
<div class="recurrent-cta">
  <button class="btn btn--block btn--dashed" hx-get="/recurrent/new"
          hx-target="#sheetContent" hx-swap="innerHTML">
    + Nuova ricorrente
  </button>
</div>
```

`partials/recurrent_monthly_overview.html` — the dashboard tile under
"Spese ricorrenti" reuses the same `.recurrent-row` markup but capped at 5
rows + a "Tutte →" chip linking to `/recurrent`.

### CSS (`web/static/css/recurrent.css`)
```css
.page-header{padding:24px 0 20px;border-bottom:1px dotted var(--line);}
.page-header__title{
  font-family:var(--font-display);font-weight:400;
  font-size:36px;letter-spacing:-0.01em;margin:6px 0 18px;
}
.recurrent-total{display:flex;align-items:baseline;gap:8px;}
.recurrent-total__int{font-family:var(--font-display);font-size:44px;letter-spacing:-0.02em;}
.recurrent-total__suffix{font-family:var(--font-display);font-size:18px;color:var(--ink-2);}

.recurrent-list{padding:20px 0 16px;}
.recurrent-row{
  display:grid;grid-template-columns:auto 1fr auto auto;
  gap:14px;align-items:center;padding:16px 0;
  border-bottom:1px dotted var(--line);
}
.recurrent-row:last-child{border-bottom:none;}
.recurrent-row__day{
  width:44px;height:44px;border-radius:4px;
  background:var(--paper-raised);border:1px solid var(--line);
  display:flex;align-items:center;justify-content:center;
  font-family:var(--font-display);font-size:16px;color:var(--ink-2);
}
.recurrent-row__name{font-family:var(--font-display);font-size:18px;}
.recurrent-row__sub{margin-top:2px;}
.recurrent-row__amt{font-family:var(--font-display);font-size:18px;}
.recurrent-row__actions{display:flex;}

.recurrent-cta{padding:16px 0 32px;}
.btn--dashed{border-style:dashed;}
```

### `partials/recurrent_form.html`
Already covered by ADR-0010 (Quaderno forms). This ADR only covers the
**list / overview** surface.

## Inputs / Outputs

| Op | Input | Output |
|---|---|---|
| Recurrent list | none | header VM + N row VMs + monthly total |
| Recurrent monthly overview tile | none | top-5 rows + link |

## Edge cases
- Zero recurrents → list area replaced by empty-state (re-uses ADR-0008
  `.empty-state`); CTA still rendered.
- Day = 31 on Feb-only schedule → `NextRunShort` honors existing handler logic.
- Amount whole-euro → `AmountFmt` shows no decimals; otherwise two decimals
  with `,` separator (Italian).
- Annual frequency total excluded from `MonthlyTotalCents` (count is
  monthly-only); annual rows still listed but flagged with `FreqLabel = "Annuale"`.

## Error conditions
- DB error → 500 + slog (existing).
- Invalid recurrent ID on edit → 404 (existing).

## Test plan
1. **Acceptance** (`internal/http/server_test.go`):
   - GET `/recurrent` after seeding three monthly + one annual recurrent →
     response includes:
     - `Spese ricorrenti` heading.
     - A `<div class="recurrent-row__day">01</div>` for a day-1 entry.
     - Total equal to sum of monthly amounts (annual excluded).
2. **Unit**:
   - `formatAmountIT(72000)` → `"720"`, `formatAmountIT(2990)` → `"29,90"`.
   - `nextRunShort(time.Date(2026,12,1,...))` → `"01 Dic"`.
3. **Integration**:
   - Editing a recurrent via `hx-get` returns the new sheet form
     (ADR-0010 markup); save round-trips and list re-renders.

## Out of scope
- Pause / resume / archive controls (separate ADR).
- Annual / weekly visual sub-grouping (current single list is enough).
- Sparklines per recurrent (future "history" panel).
