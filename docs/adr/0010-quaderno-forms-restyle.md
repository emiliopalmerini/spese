# ADR-0010: Quaderno restyle of expense, income, recurrent forms

## Status
Accepted (CSS-only restyle of existing markup; new partials form_amount/form_chips/form_field-inline + dynamic chip data deferred — current Alpine.js category-picker chips already provide the chip pattern.)

## Context
ADR-0008 (tokens) + ADR-0009 (dashboard chrome) leave the Add/Edit forms
(`partials/expense_form.html`, `partials/income_form.html`,
`partials/recurrent_form.html`, `partials/recurrent_edit_form.html`,
`partials/recurrent_edit_form_sheet.html` and the shared
`components/form_field.html`) styled with the previous brutalist pattern:
hard 2px borders, `Inter` body type, no big serif amount.

Quaderno (`v1-quaderno.jsx → QAddExpense`) shows a different pattern:

- Big centered serif amount (`80px`, `Instrument Serif`) with a leading `€`
  glyph at `30px` and a mono `EUR` caption beneath.
- Field rows: dotted-bottom row with mono uppercase tracked label on the left
  and serif value (`18px`) on the right; chevron `›` for category pickers.
- Quick-add chips (a horizontal row) for repeat amounts.
- Two-button footer: outline `Annulla` + filled ink `Registra spesa`.

This ADR ports those patterns to the three existing form partials and to
`form_field.html`, which is the shared component everything else uses.

No backend / handler changes; pure markup + CSS + a small amount of
JS-side wiring for the chips (optional in v1).

## Decision

### `components/form_field.html` (rewrite)
Two flavors driven by a `Variant` prop (defaults to `inline`):

- `inline` (default, used by Add/Edit forms): one row,
  ```html
  <label class="field field--inline">
    <span class="field__label label-mono">{{.Label}}</span>
    <span class="field__value">
      <input class="field__input" name="{{.Name}}" value="{{.Value}}" {{.Attrs}} />
      {{if .Chevron}}<span class="field__chevron">›</span>{{end}}
    </span>
  </label>
  ```
- `stacked` (used where multi-line input or textarea is needed): retains
  current vertical structure but uses the new `label-mono` class for the
  label.

### Big amount block
New partial `partials/form_amount.html`:
```html
<div class="form-amount">
  <div class="label-mono">Importo</div>
  <div class="form-amount__row">
    <span class="form-amount__cur">€</span>
    <input class="form-amount__input" name="{{.Name}}" inputmode="decimal" value="{{.Value}}" />
  </div>
  <div class="label-mono form-amount__cap">EUR</div>
</div>
```

### Quick-add chips
New partial `partials/form_chips.html`:
```html
<div class="form-chips">
  <div class="label-mono form-chips__title">Rapido</div>
  <div class="form-chips__row">
    {{range .Chips}}
      <button type="button" class="chip" data-amount="{{.Cents}}" data-desc="{{.Desc}}">{{.Label}}</button>
    {{end}}
  </div>
</div>
```
Chips populate the amount + description fields on click via a tiny inline
script (≤ 20 lines added to `expense-form.js`). Chip set is **static** for
this ADR (`Caffè 1,50 / Pranzo 12,00 / Spesa 30,00 / Benzina 50,00 /
Treno 10,00`); a follow-up ADR can derive top-N user chips from history.

### CSS (`web/static/css/forms.css`)
```css
.field--inline{
  display:flex;align-items:center;justify-content:space-between;
  gap:12px;padding:18px var(--space-5);
  border-bottom:1px dotted var(--line);
}
.field__label{flex:0 0 auto;}
.field__value{display:flex;align-items:center;gap:8px;}
.field__input{
  background:transparent;border:none;outline:none;
  font-family:var(--font-display);font-size:18px;color:var(--ink);
  text-align:right;width:200px;
}
.field__chevron{color:var(--muted);}

.form-amount{padding:36px var(--space-5) 28px;text-align:center;border-bottom:1px dotted var(--line);}
.form-amount__row{display:flex;align-items:baseline;justify-content:center;gap:6px;margin-top:14px;}
.form-amount__cur{font-family:var(--font-display);font-size:30px;color:var(--ink-2);}
.form-amount__input{
  background:transparent;border:none;outline:none;
  font-family:var(--font-display);font-size:80px;font-weight:400;
  letter-spacing:-0.03em;color:var(--ink);
  width:240px;text-align:center;padding:0;
}
.form-amount__cap{margin-top:4px;}
.form-chips{padding:20px var(--space-5) 16px;}
.form-chips__title{margin-bottom:10px;}
.form-chips__row{display:flex;gap:8px;flex-wrap:wrap;}
.form-footer{padding:12px var(--space-5) 32px;display:flex;gap:10px;}
.form-footer .btn{flex:1;}
```

### Form templates
- `partials/expense_form.html`: replace amount input with `form_amount`,
  description/category/subcategory rows with `form_field.inline`, append
  `form_chips`, footer with `Annulla` (`btn`) + `Registra spesa`
  (`btn-primary`, full-width via `btn--block` on each via flex).
- `partials/income_form.html`: same pattern, "Registra entrata" CTA.
- `partials/recurrent_form.html` + `recurrent_edit_form.html` +
  `recurrent_edit_form_sheet.html`: `form_amount` + frequency / day rows as
  `field--inline` with chevron. Submit: "Salva ricorrente".

### Bottom-sheet (`dashboard.html` already has `.bottom-sheet`)
Header gets `.label-mono` for category sub-text; title becomes
`font-family: var(--font-display); font-size: 36px; letter-spacing: -0.01em;`.
Existing CSS hooks renamed only via class additions (no removal in this ADR).

## Inputs / Outputs
Unchanged from current behavior — same form fields, same POST handlers,
same validation. Only the rendered shape changes.

## Edge cases
- Empty `Value` on `form-amount__input` → shows nothing (no placeholder); a
  subsequent ADR can add a `0,00` ghost placeholder.
- Long category names overflow `field__input` width → `text-overflow:
  ellipsis` on `field__value` ensures clean truncation.
- Chip click while amount input is focused: should overwrite, not append.
- iOS Safari numeric keyboard: `inputmode="decimal"` keeps current behavior.
- Recurrent edit "sheet" variant (`recurrent_edit_form_sheet.html`) uses
  the same partials as the page variant for consistency.

## Error conditions
- Server-side validation errors continue to flash via `flash_messages.html`
  (no change).

## Test plan
1. **Acceptance** (`internal/http/server_test.go`):
   - GET `/ui/forms/expense` returns markup with `form-amount__input` + at
     least one `chip` + `field--inline` rows.
   - POST `/expenses` with valid payload still returns 200 / triggers
     `dashboard:refresh`.
2. **Unit**:
   - `form_field.html` partial: variant=`inline` renders `field--inline`;
     variant=`stacked` renders `field--stacked`.
3. **Integration**:
   - Submit expense via bottom sheet → row appears in transactions list
     after refresh trigger.

## Out of scope
- Dynamic top-N chip generation from user history (future ADR).
- Custom category picker UI (uses current `category_selector.html`).
- Currency selector (always EUR for now).
- A11y deep-dive (keyboard focus rings already covered by
  `:focus-visible` global rule).
