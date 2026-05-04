# ADR-0008: Quaderno design tokens and global chrome

## Status
Accepted

## Context
Spese currently uses a monochrome typography-first system (Space Grotesk + Inter
+ JetBrains Mono, pure black accent, 0-radius brutalist). The "Quaderno" design
direction (`Spese Redesign.html`, variation 01 in the design bundle) reframes
the app as a **paper diary**: warm off-white background, ink-black type, a
single terracotta accent, dotted dividers between sections, big serif numerals
for monetary values, mono uppercase tracked labels. It is the "lead" variation
of the redesign and the only one with a fully designed flow (dashboard, add,
categories, recurring, onboarding, settings).

This ADR covers the **first atomic slice** of the port: design tokens, fonts,
and the cross-cutting chrome (topbar/masthead, section labels, dotted-divider
sections, button + chip styles). Per-screen ports (dashboard hero/KPI strip,
expense form, categories panel, recurrent panel) are deferred to follow-up
ADRs (0009+) so each ships a single behavior change.

No backend, storage, or service changes. Pure CSS + template chrome.

## Decision

### Design tokens (`web/static/css/base.css`)
Replace the current palette + typography vars (keep variable *names* so
existing rules stay valid):

```
/* Typography */
--font-display: 'Instrument Serif', 'EB Garamond', Georgia, serif;
--font-body:    'Geist', 'Inter', -apple-system, system-ui, sans-serif;
--font-mono:    'Geist Mono', 'JetBrains Mono', ui-monospace, monospace;

/* Quaderno palette */
--paper:         #f5f1e8;   /* page bg */
--paper-raised:  #fbf7ee;   /* card / topbar bg */
--ink:           #1a1612;
--ink-2:         #5a5650;
--muted:         #8a847a;
--line:          rgba(26,22,18,0.12);
--accent:        #b8451c;   /* terracotta */
--accent-contrast: #ffffff;
--positive:      #3d6b4a;
--negative:      #a83c2e;

/* Semantic remap */
--bg:            var(--paper);
--bg-light:      var(--paper-raised);
--surface:       var(--paper-raised);
--surface-2:     var(--paper);
--text:          var(--ink);
--text-secondary:var(--ink-2);
--border:        var(--line);
--border-strong: var(--ink);
--primary:       var(--ink);
--accent-600:    var(--accent);
--accent-700:    #9a3917;
```

Dark mode: defer (Quaderno is paper-first; `prefers-color-scheme: dark` block
is removed in this ADR; a future ADR can introduce a Notturno-style dark theme
or a Quaderno-night tweak).

`<meta name="theme-color">` updated from `#000000` to `#f5f1e8` in
`layouts/base.html` and `pages/dashboard.html`.

### Fonts
Replace the Google Fonts `<link>` in `layouts/base.html` and
`pages/dashboard.html` with:

```html
<link href="https://fonts.googleapis.com/css2?family=Geist:wght@300;400;500;600;700&family=Geist+Mono:wght@400;500;600&family=Instrument+Serif:ital@0;1&display=swap" rel="stylesheet">
```

### Mono-label utility (new in `utilities.css`)
```css
.label-mono{
  font-family:var(--font-mono);
  font-size:var(--text-xs);
  letter-spacing:0.16em;
  text-transform:uppercase;
  color:var(--muted);
}
```

### Section primitive (new in `cards.css`)
A vertical-stack layout where each section is separated by a 1px dotted ink
divider; replaces the per-section `border-bottom: 2px solid` look:

```css
.page__section + .page__section{
  border-top:1px dotted var(--line);
  padding-top:var(--space-5);
}
.section-title{
  font-family:var(--font-display);
  font-weight:400;
  font-size:var(--text-xl);
  letter-spacing:-0.01em;
  border-bottom:none;
  margin:0 0 var(--space-3);
}
```

A complementary `.section-label` block (Roman numeral + accent italic + mono
sub) is added but **only adopted by the dashboard ADR-0009**; this ADR only
defines it.

### Topbar / masthead (`topbar.css`)
- Background `var(--paper-raised)`, divider `1px solid var(--line)`.
- Brand: `font-family: var(--font-display)`, weight 400, italic on the second
  word ("Spese · *diario*") via a span — keep DOM minimal: just add
  `<span class="brand__sub">diario</span>` next to existing brand text.
- Nav links: keep current underline-on-hover behavior; switch font to mono
  uppercase tracked at `letter-spacing:0.14em`, `font-size:var(--text-xs)`.

### Buttons (`buttons.css`)
- `.btn` keeps zero radius, but border becomes `1px solid var(--ink)` (was
  `2px`) and base `background: transparent`. Hover inverts to filled.
- `.btn-primary` uses `background: var(--ink)` / `color: var(--paper-raised)`.
- New `.btn-accent` variant: `background: var(--accent)` / `color: #fff`,
  for the FAB "primary" action and onboarding CTA.
- New `.chip` class (used by period chips + quick-add chips):
  ```
  background: var(--paper-raised);
  border: 1px solid var(--line);
  border-radius: var(--radius-pill);
  padding: 8px 14px;
  font-family: var(--font-mono);
  font-size: var(--text-xs);
  ```
  `.chip--active` flips to `background: var(--ink); color: var(--paper-raised)`.

### FAB (`dashboard.css` / existing rules in `style.css`)
- Color updated from black to `var(--accent)`; ring shadow from `0 6px 16px rgba(0,0,0,0.3)` to `0 6px 16px rgba(184,69,28,0.3)`.
- `.fab__icon` stroke color follows `var(--accent-contrast)`.

### Body class
Existing `class="theme-ink density-comfortable style-minimal"` → leave the
markers in place (no Go/template change required); they are decorative until
a tweaks panel ADR is written.

## Inputs / Outputs
| Op | Input | Output |
|---|---|---|
| Render any page | existing template + new tokens | Quaderno-styled chrome (paper bg, serif headings, dotted dividers, terracotta accent, mono labels) |

## Edge cases
- **Webfont blocked / offline**: fallback chain (`'EB Garamond', Georgia, serif`
  for display; `'Inter', system-ui` for body) keeps the page legible.
- **Existing body class `theme-ink`**: untouched; CSS rules are global, not
  scoped under that class, so no JS coupling breaks.
- **Reduced motion**: existing `prefers-reduced-motion` block kept verbatim.
- **`<meta name="theme-color">`** on iOS PWA: paper color may look muddy on
  some browsers; acceptable trade-off and easily reverted.

## Error conditions
None. CSS-only.

## Test plan

1. **Acceptance** (manual + Playwright-style smoke; see `scripts/smoke.sh`):
   - Load `/` → body computed `background-color` resolves to `rgb(245, 241, 232)` (paper).
   - `.brand` computed `font-family` includes `"Instrument Serif"`.
   - `.btn-primary` computed `background-color` resolves to `rgb(26, 22, 18)` (ink).
   - FAB `background-color` resolves to `rgb(184, 69, 28)` (accent).
   - `.page__section + .page__section` computed `border-top-style` = `dotted`.
2. **Unit (CSS regression via Go test)**:
   - Add `internal/http/server_test.go` case that GETs `/static/css/base.css`
     and asserts the new tokens are present (`--paper`, `--accent: #b8451c`, `Instrument Serif`).
   - GETs `/static/css/buttons.css` and asserts `.btn-accent` rule exists.
3. **Integration**:
   - Existing dashboard server tests must still return HTTP 200 with no
     template-render errors after the layout/dashboard `<head>` changes.

## Out of scope (future ADRs)
- **0009**: Dashboard masthead + hero number + KPI strip + section-label adoption.
- **0010**: Add expense / income / recurrent forms restyled (big serif amount input, mono field labels, paper card chrome).
- **0011**: Categories breakdown panel restyled (donut center label, dotted rows).
- **0012**: Recurrent list restyled (day-card avatar, serif amounts).
- **0013**: Notturno-style dark theme tweak (re-introduce `prefers-color-scheme: dark` mapping).
- **0014**: Bottom tab bar (Quaderno-style with central round FAB) — only if mobile primary navigation is desired.
- Charts (bar / donut / sparkline) port: deferred; existing inline SVG keeps working with new colors thanks to `currentColor` usage.
