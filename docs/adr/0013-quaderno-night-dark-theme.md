# ADR-0013: Quaderno-night (dark) theme tweak

## Status
Accepted (Settings UI for theme picker deferred to ADR-0014; toggle is currently exposed only via window.SpeseTheme.set('light'|'night'|'auto').)

## Context
ADR-0008 dropped the previous `prefers-color-scheme: dark` mapping because
Quaderno is paper-first. Users who run their OS in dark mode still need a
non-blinding rendering. The Notturno variation (`v4-notturno.jsx`) is too
much of a Bloomberg-pivot to be the default; instead this ADR introduces a
**Quaderno-night** palette: warm dark paper (`#1a1612` background, the
old "ink"), parchment-cream text, the same terracotta accent.

This keeps brand identity coherent across light/dark, no font swap, no
layout swap; only token values flip.

## Decision

### Palette flip
In `web/static/css/base.css`, re-introduce a `prefers-color-scheme: dark`
block, but only mapping the semantic vars — leave the Quaderno raw tokens
(`--paper`, `--ink`, …) defined in :root for explicit light overrides:

```css
@media (prefers-color-scheme: dark){
  :root{
    --paper:         #1a1612;
    --paper-raised:  #221d18;
    --ink:           #f1ece1;
    --ink-2:         #b9b0a2;
    --muted:         #7e7768;
    --line:          rgba(241,236,225,0.14);
    --accent:        #d77a4f;        /* slightly desaturated terracotta for night */
    --accent-contrast:#1a1612;
    --positive:      #6fa37f;
    --negative:      #d4796a;

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
    --accent-700:    #b66440;
  }
  meta[name="theme-color"]{ /* documentation only */ }
}
```

Update `<meta name="theme-color">` strategy: emit two via JS or use the
`media` attribute:
```html
<meta name="theme-color" content="#f5f1e8" media="(prefers-color-scheme: light)">
<meta name="theme-color" content="#1a1612" media="(prefers-color-scheme: dark)">
```
applied in `layouts/base.html` and `pages/dashboard.html`.

### Manual override (opt-in)
Add a `data-theme` toggle on `<html>` that beats the media query:

```css
html[data-theme="light"]{ /* keep :root values; explicit class to override system dark */
  color-scheme: light;
}
html[data-theme="night"]{
  color-scheme: dark;
  /* duplicate the media-query overrides here so toggling without OS change works */
}
```

Add a Settings row "Tema · Auto / Chiaro / Notte" (links to the existing
settings surface). Storage is `localStorage["spese.theme"]`; default
`auto`. Tiny JS in `web/static/dashboard.js`:

```js
(function(){
  var t = localStorage.getItem('spese.theme') || 'auto';
  if (t === 'light' || t === 'night') {
    document.documentElement.setAttribute('data-theme', t);
  }
  window.SpeseTheme = {
    set: function(v){
      localStorage.setItem('spese.theme', v);
      if (v === 'auto') document.documentElement.removeAttribute('data-theme');
      else document.documentElement.setAttribute('data-theme', v);
    }
  };
})();
```

### Image / icon adjustments
- Favicon already monochrome SVG; `currentColor` ensures it tracks.
- FAB shadow under night theme: `0 6px 16px rgba(215,122,79,0.35)` (slightly
  brighter halo on dark bg).
- Skeleton loaders: existing `var(--bg-light)` shimmer keeps working.

## Inputs / Outputs

| Op | Input | Output |
|---|---|---|
| OS toggle (no JS) | `prefers-color-scheme: dark` | tokens flipped to night palette |
| Manual override | `SpeseTheme.set('night')` / `'light'` / `'auto'` | `data-theme` attr toggled, `localStorage` persisted |

## Edge cases
- First load before JS runs: page renders with system preference (no flash).
  When JS runs and a stored override exists, attribute is set and CSS
  re-applies (one-frame flicker possible; acceptable for v1).
- Print stylesheet: night theme should not apply when printing — wrap the
  `@media (prefers-color-scheme: dark)` rules and the manual `[data-theme="night"]`
  rules under `@media screen` (since `prefers-color-scheme` evaluates true
  in some print-preview contexts).
- High-contrast forced colors mode: rely on browser; no further work in v1.
- Embedded Google Sheets export not affected (server-side only).

## Error conditions
- `localStorage` unavailable (private mode) → fall back to auto; do not
  throw.

## Test plan
1. **Acceptance** (manual / smoke):
   - With OS dark on, load `/` → body `background-color` = `rgb(26,22,18)`,
     text color light.
   - Toggle to `light` via console: `SpeseTheme.set('light')` → body
     `background-color` = `rgb(245,241,232)`.
2. **Unit**:
   - CSS regression: `base.css` contains both `[data-theme="night"]` selector
     and `prefers-color-scheme: dark` block.
3. **Integration**:
   - Existing dashboard server tests run unchanged.

## Out of scope
- A full "Notturno" variation (lime accent, monospace, terminal feel) — that
  remains a future variation, not the default night theme.
- Sepia / e-ink tweak.
- Per-component dark adjustments beyond palette flip.
