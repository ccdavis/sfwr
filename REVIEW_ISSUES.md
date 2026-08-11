# Code Review Tracker

## 1) Cover image download condition is inverted
- Status: Fixed
- Location: `models/open_library.go:131`
- Problem: `CaptureCoverImages` downloads when `!b.HasCoverImageId()`, which is the opposite of the intended behavior; it skips books that actually have cover IDs and tries to fetch for those missing IDs.
- Impact: Missing covers and misleading logging during batch image fetches.
- Proposed fix: Flip the condition to download only when `HasCoverImageId()` is true; update the log message accordingly.

## 2) Cover image download errors are misreported and can terminate the app
- Status: Fixed
- Location: `models/open_library.go:94`
- Problem: `saveCoverImage` wraps `os.Create` and `io.Copy` errors using the wrong variable (`e`), and `log.Fatal` aborts the process on copy failures.
- Impact: Errors are obscured and the web server can exit during a background image fetch.
- Proposed fix: Wrap and return `err` consistently; replace `log.Fatal` with a non-fatal return so callers can log and continue.

## 3) Open Library search results can panic if authors are missing
- Status: Fixed
- Location: `models/open_library.go:28`
- Problem: `BookSearchResult.Print()` accesses `Authors[0]` without checking length.
- Impact: A missing author list from Open Library can panic in the TUI search flow.
- Proposed fix: Guard for empty authors and print a placeholder (e.g., "Unknown Author").

## 4) Author grouping can panic on empty surname
- Status: Fixed
- Location: `pages/lists.go:62`
- Problem: `string(a.Surname[0])` panics if `Surname` is empty.
- Impact: Any malformed or missing author data can crash static generation.
- Proposed fix: Normalize empty surnames to "UNKNOWN" before grouping; avoid indexing empty strings.

# Admin Web UI Review

## 5) Every response returned HTTP 200, including errors and unknown URLs
- Status: Fixed
- Location: `web/handlers.go` (`homeHandler`, `renderError`)
- Problem: `renderError` rendered the error page with the default 200 status, and `/` was registered as the catch-all so any unrecognised URL silently rendered the home page.
- Impact: A typo in the URL looked like a working page; browsers, `curl` and any future scripting could not distinguish success from failure.
- Fix: `homeHandler` rejects paths other than `/` with 404; `renderErrorStatus` sets 400/404/409/500 per failure. Templates now render into a buffer first so a mid-render failure can't emit a half page under a 200.

## 6) Six of nine admin pages duplicated the layout instead of using `base.html`
- Status: Fixed
- Location: `templates/web/*.html`
- Problem: Only `home`, `error` and `backups` extended `base.html`. The rest carried their own copy of the full page shell and ~190 lines of identical CSS, which had already drifted: the "Decades" nav link existed only in three files, and the author editor's stylesheet was missing rules the shared one had.
- Impact: Navigation dead-ended depending on which page you were on, and any style change had to be made seven times.
- Fix: All pages now extend `base.html`. The shared stylesheet holds the union of the previously duplicated rules, the nav includes Decades and Deployments, and the current section is highlighted.

## 7) The flash message after deleting a book was never shown
- Status: Fixed
- Location: `web/handlers.go` (`listBooksHandler`)
- Problem: `deleteBookHandler` redirected to `/books?message=Book deleted successfully`, but the books handler never read the `message` query parameter.
- Impact: Deletes gave no confirmation at all.
- Fix: The handler reads `message` and the template renders it.

## 8) The books list rendered the entire collection with no search
- Status: Fixed
- Location: `web/handlers.go` (`listBooksHandler`), `templates/web/book_list.html`
- Problem: All 464 books were rendered on one page (~435 KB) with full review text and no way to find a specific title.
- Impact: Editing a known book meant scrolling or using the browser's find.
- Fix: Title/author search, 50-per-page pagination that preserves the search and sort, a result count, cover thumbnails, and truncated review previews (~85 KB per page).

## 9) Sorting was wrong on both list pages
- Status: Fixed
- Location: `web/handlers.go` (`listBooksHandler`, `listAuthorsHandler`)
- Problem: "Sort by Author Name" ordered on `author_full_name`, i.e. by first name. The authors page had no `Order` clause at all, so 251 authors appeared in insertion order.
- Impact: Neither list could be scanned to find a writer.
- Fix: Books sort by `author_surname`; authors sort by surname then full name.

## 10) The author picker corrupted data for multi-author books
- Status: Fixed
- Location: `templates/web/book_form.html`
- Problem: The field was filled with `{{range .Book.Authors}}{{.FullName}}{{end}}` and the hidden ID with the same loop over IDs, so a two-author book produced a concatenated name and a concatenated ID (authors 1 and 2 became "12"). The visible text input was also named `author_id` and renamed by JavaScript at submit time.
- Impact: Latent data corruption — saving such a book would attach it to whatever author ID the concatenation happened to spell. No multi-author books exist today, so nothing is currently damaged.
- Fix: `Book.PrimaryAuthor()` supplies a single author; the hidden field is named `author_id` from the start and the visible box is a separate `author_name` field kept in sync, with `setCustomValidity` blocking submission of an unmatched name.

## 11) Rollback left the server using the pre-rollback database
- Status: Fixed
- Location: `web/handlers.go` (`rollbackHandler`), `web/deploy.go`
- Problem: `git checkout <commit> -- sfwr_database.db` replaces the file, but the GORM connection opened at startup kept pointing at the old one. `commitHash[:7]` in the success message also panicked on a hash shorter than seven characters.
- Impact: A rollback appeared to succeed while the UI kept showing — and re-saving — the old data.
- Fix: `reopenDatabase()` closes and reopens the connection after a successful rollback, and reports clearly if that fails. `shortHash` replaces the unchecked slice.

## 12) Deleting a book left dangling `book_authors` rows
- Status: Fixed
- Location: `web/handlers.go` (`deleteBookHandler`)
- Problem: Book deletes are soft (`gorm.DeletedAt`), so join rows survived and kept pointing at a hidden book. One such row exists in the current database.
- Impact: Stale rows accumulate on every delete.
- Fix: The authors association is cleared before the soft delete.

## 13) Deployment failures were rendered into a template with no error slot
- Status: Fixed
- Location: `web/handlers.go` (`deployHandler`), `templates/web/home.html`
- Problem: A failed deploy set `PageData.Error` and rendered `home.html`, which only rendered `.Message`.
- Impact: A failed push looked like nothing happened.
- Fix: `home.html` renders `.Error`, and the response carries a 500.

## 14) Association and validation errors were silently discarded
- Status: Fixed
- Location: `web/handlers.go` (book create/update, author update)
- Problem: `Association(...).Append/Replace/Delete` return values were ignored, book titles were stored untrimmed and unvalidated, and choosing "remove and reassign" without a destination author quietly did nothing.
- Impact: Silent partial saves that looked successful.
- Fix: Every association error is checked, titles are trimmed and required, and the reassign path reports missing selections.

## 15) Cover search only fell back to Google Books / Apple when Open Library returned nothing
- Status: Fixed
- Location: `web/handlers.go` (`searchOpenLibraryHandler`)
- Problem: The fallback triggered on `len(responseItems) == 0`, so an Open Library match with no cover art suppressed the other sources entirely.
- Impact: Books stayed coverless even though Google or Apple had artwork.
- Fix: Fall back whenever no result carries a cover, not only when there are no results.

## 16) Usability gaps in author management
- Status: Fixed
- Location: `web/handlers.go`, `templates/web/author_list.html`, `templates/web/author_edit.html`
- Problem: There was no way to delete an author; duplicate names could be created freely; the "Add Books to Author" list showed all 464 books unfiltered; and the action dropdown that decides what the checkboxes do sat below them.
- Fix: Delete is available for authors with no books (409 otherwise), creating a duplicate name redirects to the existing author, the add-books list has a client-side filter, and the action selector moved above the checkbox lists with explanatory text.

## 17) The admin UI had no authentication and bound to every interface
- Status: Fixed
- Location: `web/auth.go` (new), `web/handlers.go`, `main.go`
- Problem: `http.ListenAndServe(":"+port, ...)` listened on all interfaces with no login, no CSRF protection, and no security headers. Anyone who could reach the port could edit the collection, delete books, force a `git push`, or roll the database back.
- Impact: The UI could not be published, and even locally any web page could forge a request to it.
- Fix: A `Config` carrying the bind address, password hash, and proxy mode; a single bcrypt-verified admin password with in-memory sessions; a CSRF token on every session validated on all non-GET requests; login rate limiting; and CSP, `X-Frame-Options`, `X-Content-Type-Options`, `Referrer-Policy`, and `Cache-Control: no-store` on every response. The bind address now defaults to `127.0.0.1`, and `Config.Validate()` refuses a non-loopback bind that lacks a password or TLS.

## 18) Git and build operations were reachable by anyone who reached the port
- Status: Fixed
- Location: `web/handlers.go` (`ServeHTTP`), `web/auth.go` (`requireLocal`, `isLocalRequest`)
- Problem: `/deploy`, `/rollback`, `/build-local`, and `/backups` shell out to `git push`, `git checkout` over the live database, and `./sfwr -build`.
- Impact: Publishing the UI would have handed remote clients the ability to force-push and to overwrite the database from git history.
- Fix: `requireLocal` restricts those four routes to browsers on the machine itself, and the templates hide their controls remotely. The check requires a loopback `RemoteAddr` *and*, in proxy mode, the absence of `X-Forwarded-For` — a proxied request always carries that header, so a remote client cannot forge local access by setting it.

## 19) Documentation described the pre-migration rating values
- Status: Fixed
- Location: `CLAUDE.md`, `AGENTS.md`, `DEPLOYMENT.md`, `GETTING_STARTED.md`
- Problem: `CLAUDE.md` listed `Kindle`, `Interesting`, and `Not-Good` as current ratings; they had been migrated to `Worth-Reading` / `Could-Not-Finish` plus the `indy` and `interesting` tag columns. The docs also stated the UI must never be exposed.
- Fix: Ratings and the tag migration are documented accurately, along with the template layout rules, the CSRF requirement for new forms, the admin configuration flags, and reverse-proxy setup.

## 20) Remaining (not fixed)
- Cover images for deleted books stay in `saved_cover_images/`. Harmless while deletes are soft, but they are never reclaimed.
- Sessions are in memory, so restarting the server signs the admin out. Fine for one maintainer; a persistent store would be needed for more.
- There is one password and no user accounts, so there is no per-user audit trail. Logins and failures are written to the server log with their source address.
