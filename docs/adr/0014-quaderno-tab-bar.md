# ADR-0014: Quaderno bottom tab bar with central FAB

## Status
Accepted (read-only Settings placeholder ships theme picker hooked to ADR-0013 SpeseTheme; full settings editing deferred.)

## Context
ADR-0008 leaves the topbar as the sole navigation. On mobile, Quaderno
(`v1-quaderno.jsx → QTabBar`) replaces the topbar nav with a fixed bottom
bar carrying four tabs (Diario, Categorie, Ricorrenti, Impost.) and a
central round terracotta FAB for "+ aggiungi". The current dashboard
already has a single round FAB (`#fab` with speed-dial actions); this ADR
**folds** that FAB into the tab bar and removes the topbar nav on mobile.

The Settings tab implies a Settings page that does not yet exist; this
ADR adds a **minimal placeholder** at `/impostazioni` so the tab is not
dead. A later ADR can grow it.

## Decision

### Tab bar component
New partial `partials/tab_bar.html`:

```html
<nav class="tabbar" aria-label="Navigazione principale">
  <a class="tabbar__tab" href="/" data-tab="diario">
    <span class="tabbar__icon">I</span>
    <span class="tabbar__label">Diario</span>
  </a>
  <a class="tabbar__tab" href="/ui/categories" data-tab="categorie">
    <span class="tabbar__icon">II</span>
    <span class="tabbar__label">Categorie</span>
  </a>

  <button class="tabbar__fab" id="fab" aria-label="Aggiungi">
    <svg class="tabbar__fab-icon" viewBox="0 0 24 24"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
  </button>

  <a class="tabbar__tab" href="/recurrent" data-tab="ricorrenti">
    <span class="tabbar__icon">IV</span>
    <span class="tabbar__label">Ricorrenti</span>
  </a>
  <a class="tabbar__tab" href="/impostazioni" data-tab="impost">
    <span class="tabbar__icon">VI</span>
    <span class="tabbar__label">Impost.</span>
  </a>
</nav>
```

Active state determined server-side: handler sets a `Tab` field on the
page VM; template adds `aria-current="page"` to the matching `.tabbar__tab`.

The existing FAB markup + speed-dial actions (`#fabActions`, `.fab-action`)
are kept and detached from `pages/dashboard.html` into a new partial
`partials/fab_speed_dial.html`. The tab bar's `#fab` opens the same
speed-dial via the same JS in `dashboard.js` (no behavioral regression).

### Topbar
On viewports `< 768px`, hide `.topbar__nav` and shorten the topbar to brand
only. The tab bar replaces nav. On `>= 768px`, the topbar nav stays visible
and the tab bar is hidden — keyboard/desktop users keep their familiar
top-of-page navigation.

### Settings placeholder
- Route: `GET /impostazioni` → `pages/settings.html`.
- Content: header "Capitolo VI · Impostazioni", three groups (Account,
  Sincronizzazione, Preferenze) populated from existing config (read-only
  for now): username (from auth, if present), Google Sheets ID, sheet name,
  last sync timestamp, currency `EUR €`, theme picker hooked to ADR-0013.
- All rows reuse the `.field--inline` class introduced in ADR-0010.

### CSS (`web/static/css/tabbar.css`, new)
```css
.tabbar{
  position:fixed;left:0;right:0;bottom:0;z-index:50;
  background:var(--paper-raised);border-top:1px solid var(--line);
  padding:6px 0 calc(24px + var(--safe-bottom));
  display:flex;align-items:center;
}
.tabbar__tab{
  flex:1;display:flex;flex-direction:column;align-items:center;gap:4px;
  padding:10px 0;color:var(--muted);text-decoration:none;
}
.tabbar__tab[aria-current="page"]{color:var(--ink);}
.tabbar__icon{font-family:var(--font-display);font-size:14px;}
.tabbar__tab[aria-current="page"] .tabbar__icon{font-style:italic;}
.tabbar__label{
  font-family:var(--font-mono);font-size:9px;letter-spacing:0.16em;
  text-transform:uppercase;
}
.tabbar__fab{
  width:56px;height:56px;border-radius:50%;
  background:var(--accent);color:#fff;border:none;margin:0 6px;
  font-family:var(--font-display);font-size:28px;line-height:1;cursor:pointer;
  box-shadow:0 6px 16px rgba(184,69,28,0.3);
}
.tabbar__fab-icon{width:24px;height:24px;stroke:#fff;stroke-width:2;fill:none;}

@media (min-width:768px){ .tabbar{display:none;} }
@media (max-width:767px){ .topbar__nav{display:none;} }

main.page{padding-bottom:calc(72px + var(--safe-bottom));}
```

The existing standalone `#fab` (positioned bottom-right) is removed only
on viewports `< 768px` (where the tab-bar fab supersedes it); on desktop
it stays, since the tab bar is hidden there.

## Inputs / Outputs

| Op | Input | Output |
|---|---|---|
| Tab bar render | `Tab` slug | nav with `aria-current` on the matching tab |
| FAB tap | click | speed-dial opens (same as existing `#fab`) |
| Settings page | none | static read-only settings (placeholder) |

## Edge cases
- iOS safe-area inset bottom: padded via `var(--safe-bottom)` so the tab
  bar clears the home indicator.
- Long content scrolling beneath the tab bar: `main.page` bottom padding
  matches the tab bar height + safe area.
- Keyboard open (form focused): tab bar can occlude inputs; fix is to add
  `data-keyboard-aware` class via a small JS listener on
  `visualViewport` in a follow-up; out of scope for v1.
- Desktop / wide viewport: tab bar hidden, topbar nav visible — both
  remain reachable.

## Error conditions
- Tab slug not in `{diario, categorie, ricorrenti, impost}` → no active
  state highlighted; navigation still works.

## Test plan
1. **Acceptance** (`internal/http/server_test.go`):
   - GET `/` → response includes `<nav class="tabbar">` with five children
     (4 tabs + FAB) and `aria-current="page"` on the Diario tab.
   - GET `/recurrent` → `aria-current="page"` on the Ricorrenti tab.
   - GET `/impostazioni` → 200 with the heading "Impostazioni".
2. **Unit**:
   - Tab-active helper: `(slug, current) → bool` returns true only when
     equal.
3. **Integration**:
   - Existing FAB-driven flow (open speed-dial → "Spesa" → submit) still
     works after the markup move.

## Out of scope
- Hiding the tab bar on scroll-down / showing on scroll-up.
- Long-press on the FAB.
- Per-tab badges / counters.
- Real settings editing (currency switch, language, sheet picker) —
  placeholder only.
