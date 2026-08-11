# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

SFWR is a book recommendation web application written in Go that functions as a static site generator. It creates HTML pages for browsing science fiction books with author information, ratings, and cover images sourced from Open Library, Google Books, and Apple Books.

The Go toolchain is pinned in `go.mod` (`go 1.26.5`); `GOTOOLCHAIN=auto` downloads it on demand, so no system Go upgrade is required. `.github/workflows/deploy.yml` pins the same version.

## Common Commands

### Building and Running
- **Build executable**: `go build -o sfwr`
- **Build for a server**: `CGO_ENABLED=0 go build -ldflags="-s -w" -o sfwr` — produces a static, dependency-free binary (~15 MB) that runs on any Linux x86-64 host regardless of its glibc version. Copy it over with `scp`; the target needs no Go toolchain.
- **Create database**: `./sfwr -createdb sfwr_database.db`
- **Download cover images**: `./sfwr -getimages`
- **Generate static site**: `./sfwr -build` (outputs to `output/public/`)
- **Add new book (TUI)**: `./sfwr -new`
- **Start admin server (local)**: `./sfwr -web=8080`
- **Generate an admin password hash**: `./sfwr -hash-password`

### Testing
- **Run all tests**: `go test ./...`
- **Run single package tests**: `go test ./models` or `go test ./web`
- **Run specific test**: `go test ./models -run TestBookCreate`
- **Verbose output**: `go test -v ./...`

Tests use in-memory SQLite databases (`:memory:`) — see `setupTestDB()` helper functions in test files. `web/auth_test.go` covers authentication, CSRF, login throttling, and the local-only gate; it builds a `WebServer` directly rather than through `NewWebServer` so it does not need the template files.

### Development
- **Run without building**: `go run main.go [flags]`
- **Install dependencies**: `go mod tidy`

Handlers are invoked directly in tests, bypassing middleware. Anything enforced in middleware (auth, CSRF, the local-only gate) will not apply to those tests — that is deliberate, and it is why the middleware has its own tests.

## Architecture

### Core Components
- **config/**: The `sfwr.conf` settings file — paths, site name, admin server options
- **models/**: Data structures and database operations using GORM
  - `book.go`: Book/Author models, Rating enum, Open Library data sync
  - `book_render.go`: Template display helpers (`StarRating`, `TagIcons`, `FormatPubDate`, review previews)
  - `open_library.go`, `google_books.go`, `itunes_search.go`, `cover_search.go`: Metadata and cover search
- **pages/**: Renders individual pages of the static site
- **site/**: Drives a whole site build — called by both `-build` and the admin UI's Rebuild button
- **templates/**: HTML templates for static site generation
  - `templates/web/`: Templates for the web admin UI
- **web/**: HTTP handlers for CRUD operations, authentication, and deployment
  - `handlers.go`: Routing, page handlers, template rendering
  - `auth.go`: Config, sessions, CSRF, login throttling, security headers
  - `deploy.go`: Git-backed deploy, rollback, and backup history
- **tui/**: Terminal interface for adding books (alternative to web UI)

### Configuration

Nothing is hardcoded to one collection. `config.Load` reads `sfwr.conf` (or `-config PATH`)
with simple `key = value` lines: `database`, `cover_images`, `output`, `templates`, `repo`,
`site_name`, `bind`, `port`, `behind_proxy`. Relative paths resolve against the config
file's own directory, so the program can be started from any working directory — which is
how a service runs it. Missing keys fall back to `config.Defaults()`, which matches a fresh
checkout, so running from the repo with no config file still works.

Flags override the file: `-bind`, `-output`, `-web` (port), `-behind-proxy`. The admin
password is env-only (`SFWR_ADMIN_PASSWORD_HASH`) and deliberately not a config key, so the
hash never sits in a file next to a document root.

`output` is the document root of the published site. Pointing it at a web server's
directory means a rebuild publishes immediately, with no git round-trip. `repo` is
optional: leave it blank and the Git deploy/rollback features disable themselves and
vanish from the UI.

### List ordering must be total and stable

Every list on the public site orders by **publication year, then title, then author**,
falling through to the record ID. The comparators in `pages/lists.go` use
`sort.SliceStable` and never tie on a single field.

This matters more than it looks. The sorts originally compared only the primary key
with `sort.Slice`, which is not stable — hundreds of books share a publication year, so
their relative order was arbitrary and *changed on every build*. Rebuilding without
touching any data reshuffled the home page and the grid, which reads as a layout bug.
`pages/pages_test.go` asserts the ordering and that it does not depend on input order.

### Rendering errors must not be fatal

`pages` renderers return `(string, error)` and `site.Generate` propagates it. They used to
call `log.Fatal`. That was survivable when only the CLI built the site, but the admin
server now builds **in-process** — a bad template would take the running server down.
Keep it that way: never add a `log.Fatal` to a code path reachable from a handler.

### Admin UI templates
Every page extends `templates/web/base.html`, which holds the entire stylesheet and the nav. Page templates start with `{{template "base.html" .}}` followed by `{{define "content"}}…{{end}}`. Do not add a page that carries its own `<html>` shell — several used to, and the copies drifted apart.

`PageData` (in `handlers.go`) is the single struct passed to every template. `renderTemplateStatus` fills in the per-request fields (`CSRFToken`, `Authenticated`, `AuthEnabled`, `LocalRequest`) automatically, so handlers only set page content. Adding a page means adding its name to `pageTemplates`.

Every form that POSTs must include:
```html
<input type="hidden" name="{{.CSRFFieldName}}" value="{{.CSRFToken}}">
```
Inside a `{{range}}`, reach the token with `{{$.CSRFFieldName}}` / `{{$.CSRFToken}}`. JSON `fetch()` calls send the token as an `X-CSRF-Token` header instead; `book_form.html` puts it in a `CSRF_TOKEN` constant.

### Data Flow
1. Books can be added via TUI (`-new`), the admin UI (`-web`), or imported from JSON (`-createdb`)
2. Open Library, Google Books, and Apple Books provide cover images and metadata (searched by title/author)
3. Cover images stored in `saved_cover_images/`, copied to output on build
4. `./sfwr -build` generates static HTML in `output/public/`
5. Deployment: push database to GitHub → GitHub Actions builds and deploys to Pages

### Database
SQLite via `github.com/glebarez/sqlite`, a **pure-Go** GORM driver over `modernc.org/sqlite`.

This is deliberate. The original `gorm.io/driver/sqlite` wraps `mattn/go-sqlite3`, which needs
cgo, which links the build machine's glibc — a binary built on Ubuntu 24.04 (glibc 2.39) is
not reliably runnable on a 22.04 server (glibc 2.35), and its DNS resolution is the first
thing to break. The pure-Go driver builds with `CGO_ENABLED=0` into a static binary and uses
Go's own resolver, so deployment is a plain file copy. Don't reintroduce a cgo dependency
without a reason; it turns releases back into a toolchain-matching exercise.

Key tables:
- **books**: Core entity with rating, review, tags, cover source, Open Library IDs
- **authors**: Many-to-many with books via `book_authors` join table
- **open_library_book_isbns**, **open_library_book_authors**: External API data

Deletes are soft (`gorm.Model` supplies `deleted_at`). Because of that, code that deletes a book must clear its `Authors` association first, or the join rows outlive the book.

### Rating System
String enum stored in the `rating` column: `Excellent`, `Very-Good`, `Worth-Reading`, `Could-Not-Finish`, plus `Not Rated` for unset. Numeric values run 4 down to 1 and drive the star display.

The older `Kindle`, `Interesting`, and `Not-Good` values were replaced by ratings plus two boolean tag columns, `indy` and `interesting`. `./sfwr -migrate` converts legacy rows (`Kindle` → Indy + Very-Good, `Interesting` → Interesting + Worth-Reading, `Not-Good` → Could-Not-Finish). `convertLegacyRating` still accepts the old strings when importing old JSON.

## Admin Web UI

The admin UI runs as a normal website: it can be published on a domain, and it authenticates.

### Configuration
| Flag / variable | Purpose |
| --- | --- |
| `-config=PATH` | Settings file. Defaults to `./sfwr.conf` if present. |
| `-web=PORT` | Port to listen on. Overrides `port` in the config file. |
| `-bind=ADDR` | Interface to listen on. **Defaults to `127.0.0.1`.** Use `0.0.0.0` to accept remote connections. |
| `-output=DIR` | Where a build writes the static site. Overrides `output`. |
| `-base-path=/admin` | Serve the UI under a sub-path. Overrides `base_path`. |
| `-behind-proxy` | The server sits behind a TLS-terminating reverse proxy: marks cookies `Secure` and reads the client address from `X-Forwarded-For`. |
| `SFWR_ADMIN_PASSWORD_HASH` | bcrypt hash of the admin password. Set it with `./sfwr -set-password`, which writes `sfwr.env` for you; `-hash-password` only prints the line. |

`Config.Validate()` refuses to start when the configuration would expose the UI unsafely:
- non-loopback bind with no password → refused
- non-loopback bind without `-behind-proxy` → refused (credentials would cross the network in cleartext)

A loopback bind with no password is still allowed, which keeps `./sfwr -web=8080` working for local editing.

### Publishing it
```bash
./sfwr -hash-password                       # prints the export line
export SFWR_ADMIN_PASSWORD_HASH='$2a$12$…'
./sfwr -web=8080 -bind=127.0.0.1 -behind-proxy
```
with a proxy in front:
```
admin.example.com {
    reverse_proxy 127.0.0.1:8080
}
```
Binding to `127.0.0.1` and letting the proxy reach it over loopback is preferred over `-bind=0.0.0.0`, which would let clients skip the proxy and reach the plaintext port directly.

### Security model
- **Setting the password**: `-set-password` prompts and writes `sfwr.env` itself (atomic rename, mode 0600). It deliberately does not ask the user to redirect output: `sfwr -hash-password > sfwr.env` truncates the file before the command runs, so a mistyped password would destroy the existing one and lock the admin out. Keep it that way.
- **Sessions**: one admin password, bcrypt-verified. Sessions live in memory (a restart logs you out), last 12 hours with sliding renewal, and the cookie is `HttpOnly`, `SameSite=Lax`, and `Secure` under `-behind-proxy`. Logging in issues a fresh token so a planted session cannot be upgraded.
- **CSRF**: every session carries a token; all non-GET requests must present it. Anonymous visitors get a session too, so the login form is covered.
- **Login throttling**: 5 failures from one address triggers a 15-minute lockout. Behind a proxy the address comes from the *last* `X-Forwarded-For` entry — the one the proxy appended. Earlier entries are attacker-controlled.
- **Local-only route**: only `/rollback` — it replaces the live database with an older copy and cannot be undone from the browser. `requireLocal` restricts it to a browser on the machine itself and the buttons are hidden remotely. The check requires a loopback `RemoteAddr` *and*, in proxy mode, no `X-Forwarded-For` header, so it cannot be forged. Rebuilding, deploying, and viewing history are available to any signed-in session, since hosting the UI is the point.
- **Headers**: CSP, `X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy`, and `Cache-Control: no-store` on every response.

When adding a route, register it on the inner `mux` in `ServeHTTP` — that mux is wrapped by `authenticate`. Only `/login` is registered outside the gate.

### Base path — every emitted URL must be prefixed

`base_path` mounts the whole UI under a sub-path (`/admin`). `ws.mount` strips the prefix
before routing, so **handlers, route patterns, and `r.URL.Path` always see plain paths**
like `/books` — nothing inbound needs to know about the mount.

Outbound is the opposite: anything the server *emits* has to be prefixed, or it points
outside the mount and 404s.

- **Templates**: use the `url` helper — `href="{{url "/books"}}"`, and for a dynamic
  segment `href="{{url "/books/edit/"}}{{.ID}}"`. Never write a bare `href="/books"`.
- **Handlers**: `http.Redirect(w, r, ws.config.URL("/books"), ...)`.
- **JavaScript**: templates set `const BASE = {{.BasePath}}`; use `fetch(BASE + '/path')`.

A missed URL is invisible when `base_path` is empty and only breaks in production. The
check that catches it: request every page and grep for `href|action|src="/…"` that does
not start with the base path. `web/auth_test.go` covers the mount, the bare-prefix
redirect, off-mount 404s, the login redirect, and cookie scoping.

The session cookie is scoped to `BasePath + "/"`, so an admin UI at `/admin` does not
send its cookie to the rest of the site sharing that domain.

## Keeping two installations in sync

Editing happens in two places — the local checkout and the published server — and a
SQLite file cannot be merged, so `sync.sh` copies the whole database in one direction
at a time:

```bash
./sync.sh status   # compare, change nothing
./sync.sh push     # local -> server, then restart and rebuild there
./sync.sh pull     # server -> local
```

`remote_host` and `remote_dir` in `sfwr.conf` say where the other end is.

The comparison is `sfwr -fingerprint`, which hashes every field that reaches the
published site plus the cover files by name and size (`models/fingerprint.go`). It
deliberately ignores row order and SQLite's on-disk layout, so a VACUUM or page
rewrite does not read as a change. Both installations run the same binary, so the
check costs one ssh round trip rather than copying the database to compare it.

`.sfwr-sync-state` records both fingerprints after each successful sync. That is what
lets the script tell *which* side changed:

- one side changed → copy that direction
- **both** changed → refuse, because one side's edits would be lost; `--force` after
  you have decided which side wins
- pushing when only the server changed (or pulling when only local changed) also
  refuses, since it would discard the newer side

The destination database is always copied to `backups/` first, so even a forced sync
is recoverable.

A push restarts the admin server before rebuilding. It has to: the running server
holds the old database file open, and would otherwise keep serving — and re-saving —
the data that was just replaced. This is the same hazard as `rollback`.

## Deployment

GitHub Actions workflow (`.github/workflows/deploy.yml`) triggers on pushes to `sfwr_database.db` or `saved_cover_images/`. The workflow builds the Go executable, runs `./sfwr -build`, and deploys `output/public/` to GitHub Pages.

The admin UI's "Deploy" button commits the database and pushes to trigger the workflow. "Deployment History" lists `[DEPLOY]` commits and can roll the database back to any of them; rollback reopens the database connection afterwards, because `git checkout` replaces the file the server had open.

## Book Metadata Integration
- Search by title and author against Open Library, returning multiple potential matches
- Falls back to Google Books and Apple Books when Open Library returns nothing *or* when none of its matches have cover art
- Fetches cover images in S/M/L sizes using OLID or cover ID, saved as `saved_cover_images/book_<ID>-<S|M|L>.jpg`
- The admin UI allows selecting a search result to auto-populate book metadata
