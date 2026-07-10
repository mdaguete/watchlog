# Frontend

HTML + HTMX + Tailwind CSS (CDN). No JS frameworks.

## Design Principles

- Minimalist editorial design
- White background, serif font for titles, sans-serif for UI
- UI text in uppercase with wide letter-spacing
- No emojis or saturated colors in UI

## Templates

- Go `html/template` with pattern `{{template "head" .}}` at start and `{{template "foot" .}}` at end
- Do NOT use shared `{{template "content"}}` — Go templates have global namespace
- One template per page (`web/templates/*.html`)
- Layout defines `head` and `foot` in `web/templates/layout.html`
- All templates receive `.Lang` in their data map

## HTMX Patterns

- Handlers detect `HX-Request` header to return HTML fragment vs full page
- Infinite scroll on dashboard via `hx-trigger="revealed"` + partial templates
- Favorite/archive buttons use `hx-swap="outerHTML"` for in-place toggle
- Episode grids use vanilla JS with `fetch()` for mark/unmark — not htmx (`htmx.ajax` doesn't send JSON body correctly)

## Dark Mode

- Tailwind `darkMode: 'class'` strategy
- User preference stored in DB (`users.theme`: `system`, `light`, `dark`)
- Theme cookie set on login and settings save (JS reads it before render)
- CSS overrides in `<style>` block invert key utility colors for dark
- Calendar heatmap uses dedicated `cal-*` classes for both modes
- No flash: theme script in `<head>` runs before body paint
- Settings page: Sistema / Claro / Oscuro radio selector

## PWA (Progressive Web App)

- Installable on iOS (Add to Home Screen) and Android (native install prompt)
- Web App Manifest: standalone display, theme color black, orientation portrait
- Service Worker: network-first strategy, caches static assets for offline
- SW served at `/sw.js` (root scope)
- Install banner: auto-detected on Android (`beforeinstallprompt`), manual instructions on iOS
- Banner dismissible, persisted in localStorage
- Maskable icon (512x512 with safe zone) for Android adaptive icons
- Apple touch icon (180x180) for iOS home screen
- App shortcuts on long-press (Android): Series, Películas, Buscar, Añadir
- `viewport-fit=cover` for notch devices

## i18n (Internationalization)

- Supported languages: Spanish (es) and English (en)
- Translation function `T(lang, key)` registered in template FuncMap
- Language detection: user DB preference > Accept-Language header
- User can change language in `/settings`
- Translations stored in `internal/i18n/i18n.go` as `map[string]map[string]string`

## Pages

### Dashboard (Continue Watching)

- Shows next unwatched episode for the 5 most recently active shows
- Cards display: episode still image, show name, episode title, synopsis
- "Mark as watched" button fades out the card without page reload
- Infinite scroll (5 items per page)
- Skips episodes with no `air_date` or future air dates (not yet available)
- **Novedades** section: horizontal scroll of shows with new seasons available
- **Snooze**: hides a show until next episode is marked (button appears after 10+ days inactive)

### History (/timeline, /stats, /calendar)

The History section has two tabs: **Timeline** and **Stats**.

- **Timeline** (`/timeline`): infinite scroll of watched activity day by day.
  Day boxes connected to a central line; collapsed by default (first item +
  "+N more"). Year/month jump filter, Today button. Items link to show/movie
  detail. Localized titles.
- **Stats** (`/stats`): summary counters, total watch time, and a monthly
  **activity heatmap**. Each month cell links to that month's **Calendar**.
- **Calendar** (`/calendar?month=YYYY-MM`): editable monthly calendar reached
  by clicking a month in the Stats heatmap; prev/next month navigation.

### Search & Add (/search)

- A single box searches the **local library and TMDB** at once.
- Local matches link to their pages; TMDB matches not already in the library
  show an **Add** button (adds show/movie from TMDB). `/add` redirects here.

### Viewing history import (/import/history)

- Upload a Netflix `ViewingHistory.csv`; the analysis is persisted as a batch.
- Review page: paginated proposed watched-date changes with per-row select and
  inline date editing (HTMX); Apply writes selected changes after a DB backup.
- Series/movies not found can be reconciled against TMDB and added with their
  dates. Linked from the user settings page.

### Movies

- Two sections: Watchlist (pending, horizontal scroll) + Watched (grid)
- Adding a movie = adds to watchlist (pending)
- Mark as watched from movie detail page
- Movie detail page: poster, title, genres, runtime, synopsis, watched toggle
- Movies page has stats header (total count + runtime)

## Email Templates

- HTML email template wrapper (`wrapEmailHTML`) for consistent branding
- Inline CSS (email clients don't support external stylesheets)
- Table-based layout: WatchLog header, content area, gray footer
- Used for magic links and password reset emails

## Security (Frontend-related)

- Cookie flags: `HttpOnly`, `Secure`, `SameSite=Lax`
- All path parameters validated with `parsePathID` helper (returns 400 on invalid input)
- HTML responses with dynamic content use `html.EscapeString` (XSS prevention)
- Zip extraction uses `io.LimitReader` (100MB per file) and `filepath.Base` (path traversal prevention)
- HTTP server configured with Read/Write/Idle timeouts
- Security headers middleware: X-Content-Type-Options, X-Frame-Options, Referrer-Policy, Permissions-Policy
- Magic link authentication: passwordless login via email (token valid 1 hour, single use)
- Password reset via magic link email
- Admin-configurable auth: enable/disable registration, password login, magic link login
- Minimum password length: 8 characters

## Key Files

| File | Responsibility |
|------|---------------|
| `cmd/server/main.go` | Template loading, FuncMap, routes |
| `internal/handlers/handlers.go` | HTTP handlers (API JSON + HTML pages) |
| `internal/i18n/i18n.go` | Translation maps and language detection |
| `internal/mail/mail.go` | SMTP email sending |
| `internal/ratelimit/ratelimit.go` | Login rate limiting (per-IP) |
| `web/templates/layout.html` | `head` and `foot` (nav + footer) |
| `web/templates/*.html` | One template per page |
| `web/templates/settings.html` | Language + theme preference page |
