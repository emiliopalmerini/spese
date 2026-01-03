# Mobile-First UI Modernization Plan

**Branch:** `feat/ui-mobile-first-redesign`
**Status:** Planned, not yet implemented

## Overview
Comprehensive mobile-first redesign of the Spese expense tracking app.

## Scope
- Bottom navigation (thumb-friendly)
- PWA support (manifest, service worker, installable)
- Form UX improvements (floating labels, better inputs)
- Touch interactions (swipe to delete)
- Skeleton loading states
- CSS restructuring (true mobile-first)
- iOS/Android optimizations

**Not included:** Dark mode

---

## Phase 1: CSS Architecture (Mobile-First Breakpoints)

**File:** `web/static/style.css`

Convert from desktop-first (`max-width`) to mobile-first (`min-width`):
- Move all mobile styles to base (no media query)
- Use `@media (min-width: 560px)` for tablet/desktop
- Refactor: navigation, expense grids, recurrent items, inline editing

**Key sections to refactor:**
- Lines 66-149: Navigation
- Lines 460-528: Expense grid
- Lines 680-833: Recurrent items
- Lines 834-1135: Inline editing

---

## Phase 2: Bottom Navigation

**Files:**
- `web/static/style.css` - Add bottom nav styles
- `web/templates/layouts/base.html` - Add bottom nav HTML
- All page templates - Include bottom nav with active state

**Implementation:**
- Fixed bottom bar with 3 nav items (Spese, Ricorrenti, Entrate)
- SVG icons + labels
- 56px height + safe-area-inset-bottom
- Hide top nav on mobile, show bottom nav
- Hide bottom nav on desktop, show top nav

---

## Phase 3: PWA Support

**New files:**
- `web/static/manifest.json` - App manifest
- `web/static/sw.js` - Service worker (network-first with cache fallback)
- `web/static/icons/icon-192.png` - App icon
- `web/static/icons/icon-512.png` - App icon
- `web/static/icons/apple-touch-icon.png` - iOS icon (180x180)

**Modified files:**
- All page templates - Add PWA meta tags, manifest link, SW registration
- `internal/http/server.go` - Add route for `/sw.js` at root scope

**Meta tags to add:**
```html
<link rel="manifest" href="/static/manifest.json">
<meta name="apple-mobile-web-app-capable" content="yes">
<meta name="apple-mobile-web-app-status-bar-style" content="black-translucent">
<meta name="mobile-web-app-capable" content="yes">
```

---

## Phase 4: Form UX Improvements

**Files:**
- `web/static/style.css` - Floating label styles
- `web/templates/components/form_field.html` - Support floating variant

**Changes:**
- Add `.field--floating` class with animated labels
- Increase input min-height to 48px
- Custom select dropdown arrow
- Input groups with currency prefix for amount fields
- Font-size 16px to prevent iOS zoom

---

## Phase 5: Touch Interactions

**New file:** `web/static/swipe.js` - Swipe-to-delete handler

**CSS additions:**
- `[data-swipeable]` styles
- Swipe delete background reveal
- Touch-action constraints

**Implementation:**
- Add `data-swipeable` to expense items
- JavaScript handles touch events
- Reveals delete action on swipe left
- Integrates with existing HTMX delete

---

## Phase 6: Skeleton Loading States

**New file:** `web/templates/components/skeleton.html`

**CSS additions:**
- `.skeleton` base class with pulse animation
- `.skeleton-expense` card layout
- `.skeleton-text` variants (sm, lg)

**Template updates:**
- Replace "Caricamento..." text with skeleton components
- Update: index.html, recurrent.html, income.html

---

## Phase 7: Platform Optimizations

**CSS additions to style.css:**

iOS:
- `-webkit-fill-available` height fix
- Disable bounce in standalone mode
- Safe area handling for notch/home indicator

Android:
- Ripple effect on buttons (pseudo-element)
- Theme color meta tag

Both:
- Disable touch callout on interactive elements
- Prevent phone number auto-detection
- viewport-fit=cover

---

## Files Summary

### Modified
| File | Changes |
|------|---------|
| `web/static/style.css` | Mobile-first refactor, bottom nav, floating labels, skeletons, platform fixes |
| `web/templates/layouts/base.html` | Bottom navigation HTML |
| `web/templates/pages/index.html` | PWA meta, skeleton placeholders, SW registration |
| `web/templates/pages/recurrent.html` | PWA meta, skeleton placeholders, SW registration |
| `web/templates/pages/income.html` | PWA meta, skeleton placeholders, SW registration |
| `web/templates/components/form_field.html` | Floating label support |
| `internal/http/server.go` | Service worker route |

### New
| File | Purpose |
|------|---------|
| `web/static/manifest.json` | PWA manifest |
| `web/static/sw.js` | Service worker |
| `web/static/swipe.js` | Swipe gesture handler |
| `web/static/icons/*.png` | App icons (192, 512, apple-touch) |
| `web/templates/components/skeleton.html` | Skeleton loading components |

---

## Implementation Order

1. **CSS mobile-first refactor** - Foundation for everything else
2. **Bottom navigation** - Core mobile UX improvement
3. **PWA files** - manifest.json, sw.js, icons
4. **PWA integration** - Meta tags, SW registration, server route
5. **Form improvements** - Floating labels, better inputs
6. **Skeleton loading** - Better perceived performance
7. **Swipe interactions** - Enhanced touch UX
8. **Platform optimizations** - iOS/Android specific fixes

---

## Testing Checklist
- [ ] iPhone Safari (various sizes)
- [ ] Android Chrome
- [ ] PWA install on both platforms
- [ ] Offline functionality
- [ ] Touch targets >= 44px
- [ ] Safe areas work correctly
- [ ] Lighthouse PWA audit

---

## To Resume Implementation

```bash
git checkout feat/ui-mobile-first-redesign
# Then ask Claude to implement the plan starting from Phase 1
```
