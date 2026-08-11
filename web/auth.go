package web

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	// PasswordHashEnvVar holds a bcrypt hash of the admin password. Generate
	// one with `sfwr -hash-password`.
	PasswordHashEnvVar = "SFWR_ADMIN_PASSWORD_HASH"

	sessionCookieName = "sfwr_session"
	csrfFormField     = "csrf_token"
	csrfHeaderName    = "X-CSRF-Token"

	// sessionLifetime is how long a login lasts. Activity extends it.
	sessionLifetime = 12 * time.Hour

	// Login throttling: after maxLoginFailures bad passwords from one
	// address, that address is refused until loginLockout has passed.
	maxLoginFailures = 5
	loginLockout     = 15 * time.Minute
	loginFailWindow  = 15 * time.Minute
)

// Config carries the deployment settings that decide how the admin site
// behaves on a network.
type Config struct {
	// BindAddr is the interface to listen on. Defaults to loopback.
	BindAddr string

	// Port is the TCP port to listen on.
	Port string

	// PasswordHash is the bcrypt hash of the admin password. When empty,
	// authentication is disabled — only permitted on a loopback bind.
	PasswordHash string

	// BehindProxy tells the server it sits behind a TLS-terminating reverse
	// proxy: cookies are marked Secure and the client address is read from
	// X-Forwarded-For.
	BehindProxy bool

	// Paths, all resolved by the config package before they get here.
	DatabasePath   string
	CoverImagesDir string
	OutputDir      string

	// Templates is the template set: compiled into the binary, or a
	// directory when one is configured.
	Templates fs.FS

	// TemplatesDir is only what the config named, for the startup banner.
	TemplatesDir string

	// RepoDir is the git working tree deploy and rollback operate on.
	// Empty disables both.
	RepoDir string

	// SiteName labels the collection in the admin UI.
	SiteName string

	// ConfigFile is shown in the UI so it is clear which settings are live.
	ConfigFile string

	// BasePath mounts the whole UI under a sub-path, e.g. "/admin". Empty
	// serves from the root. Normalized by the config package.
	BasePath string
}

// URL prefixes an internal path with the configured mount point. Every URL
// the server emits — links, form actions, redirects, fetch targets — has to
// go through here, or it will point outside the mount when one is set.
func (c Config) URL(path string) string {
	if c.BasePath == "" {
		return path
	}
	if path == "/" {
		return c.BasePath + "/"
	}
	return c.BasePath + path
}

// DeployEnabled reports whether git-backed deploy and rollback are available.
func (c Config) DeployEnabled() bool {
	return c.RepoDir != ""
}

// AuthEnabled reports whether a password has been configured.
func (c Config) AuthEnabled() bool {
	return c.PasswordHash != ""
}

// Validate rejects combinations that would expose the admin UI unsafely.
func (c Config) Validate() error {
	if c.Port == "" {
		return fmt.Errorf("no port specified")
	}
	// A reverse proxy reaches the server over loopback, so binding to
	// loopback is not a protection here: whatever the proxy exposes is
	// public. This must be checked before the loopback shortcut below.
	if c.BehindProxy && !c.AuthEnabled() {
		return fmt.Errorf(
			"refusing to serve behind a reverse proxy without a password: the proxy connects over loopback, "+
				"so the admin UI would be reachable from the internet with no login.\n"+
				"Set %s (generate one with `sfwr -hash-password`), or drop -behind-proxy / behind_proxy if no proxy is in front",
			PasswordHashEnvVar)
	}
	if isLoopbackAddr(c.BindAddr) {
		return nil
	}
	// Past this point the UI is reachable from other machines.
	if !c.AuthEnabled() {
		return fmt.Errorf(
			"refusing to serve on %s without a password: set %s (generate one with `sfwr -hash-password`), or bind to 127.0.0.1",
			c.BindAddr, PasswordHashEnvVar)
	}
	if !c.BehindProxy {
		return fmt.Errorf(
			"refusing to serve on %s over plain HTTP: passwords and session cookies would cross the network in the clear.\n"+
				"Put a TLS-terminating reverse proxy in front and pass -behind-proxy, or bind to 127.0.0.1 and reach it through an SSH tunnel",
			c.BindAddr)
	}
	return nil
}

// ListenAddr is the address passed to net.Listen.
func (c Config) ListenAddr() string {
	return net.JoinHostPort(c.BindAddr, c.Port)
}

// isLoopbackAddr reports whether a bind address only accepts connections
// from this machine. An empty address means "all interfaces".
func isLoopbackAddr(addr string) bool {
	if addr == "" {
		return false
	}
	if addr == "localhost" {
		return true
	}
	ip := net.ParseIP(addr)
	return ip != nil && ip.IsLoopback()
}

// MinPasswordLength is the shortest admin password accepted.
const MinPasswordLength = 12

// HashPassword produces the bcrypt hash to put in SFWR_ADMIN_PASSWORD_HASH.
func HashPassword(password string) (string, error) {
	if len(password) < MinPasswordLength {
		return "", fmt.Errorf("password must be at least %d characters", MinPasswordLength)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// session is one logged-in browser. Anonymous sessions exist too: they
// carry a CSRF token so the login form itself can be protected.
type session struct {
	csrfToken     string
	authenticated bool
	expires       time.Time
}

// sessionStore keeps sessions in memory. Restarting the server logs
// everyone out, which is the right trade for a single-maintainer tool.
type sessionStore struct {
	mu       sync.Mutex
	sessions map[string]*session
}

func newSessionStore() *sessionStore {
	return &sessionStore{sessions: make(map[string]*session)}
}

func (s *sessionStore) get(token string) (*session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, ok := s.sessions[token]
	if !ok {
		return nil, false
	}
	if time.Now().After(sess.expires) {
		delete(s.sessions, token)
		return nil, false
	}
	return sess, true
}

func (s *sessionStore) create(authenticated bool) (string, *session, error) {
	token, err := randomToken()
	if err != nil {
		return "", nil, err
	}
	csrfToken, err := randomToken()
	if err != nil {
		return "", nil, err
	}

	sess := &session{
		csrfToken:     csrfToken,
		authenticated: authenticated,
		expires:       time.Now().Add(sessionLifetime),
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[token] = sess
	return token, sess, nil
}

func (s *sessionStore) delete(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, token)
}

// touch extends a session's lifetime so an active user isn't logged out
// mid-edit.
func (s *sessionStore) touch(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.sessions[token]; ok {
		sess.expires = time.Now().Add(sessionLifetime)
	}
}

// sweep drops expired sessions so the map can't grow without bound.
func (s *sessionStore) sweep() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for token, sess := range s.sessions {
		if now.After(sess.expires) {
			delete(s.sessions, token)
		}
	}
}

func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// loginThrottle slows down password guessing by locking out an address
// after repeated failures.
type loginThrottle struct {
	mu       sync.Mutex
	failures map[string]*failureRecord
}

type failureRecord struct {
	count     int
	lastFail  time.Time
	lockedTil time.Time
}

func newLoginThrottle() *loginThrottle {
	return &loginThrottle{failures: make(map[string]*failureRecord)}
}

// blockedFor reports how long the address must wait, or zero if it may try.
func (t *loginThrottle) blockedFor(addr string) time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()

	rec, ok := t.failures[addr]
	if !ok {
		return 0
	}
	if remaining := time.Until(rec.lockedTil); remaining > 0 {
		return remaining
	}
	return 0
}

func (t *loginThrottle) recordFailure(addr string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	rec, ok := t.failures[addr]
	if !ok || now.Sub(rec.lastFail) > loginFailWindow {
		rec = &failureRecord{}
		t.failures[addr] = rec
	}
	rec.count++
	rec.lastFail = now
	if rec.count >= maxLoginFailures {
		rec.lockedTil = now.Add(loginLockout)
		rec.count = 0
	}
}

func (t *loginThrottle) recordSuccess(addr string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.failures, addr)
}

func (t *loginThrottle) sweep() {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	for addr, rec := range t.failures {
		if now.After(rec.lockedTil) && now.Sub(rec.lastFail) > loginFailWindow {
			delete(t.failures, addr)
		}
	}
}

// clientIP is the address login throttling is keyed on. Behind a reverse
// proxy every connection comes from the proxy, so the real client is the
// last entry the proxy appended to X-Forwarded-For. Earlier entries are
// attacker-controlled and must not be trusted.
func (ws *WebServer) clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if !ws.config.BehindProxy {
		return host
	}
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		if last := strings.TrimSpace(parts[len(parts)-1]); last != "" {
			return last
		}
	}
	return host
}

// isLocalRequest reports whether the request came from this machine
// directly, bypassing any reverse proxy. Deploy and rollback are gated on
// it: they run git push and overwrite the database from git history, so
// they stay at the console even when the rest of the UI is on the network.
//
// A proxied request always carries X-Forwarded-For, so requiring the header
// to be absent means a remote client cannot forge local privileges by
// setting it themselves.
func (ws *WebServer) isLocalRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return false
	}
	if ws.config.BehindProxy && r.Header.Get("X-Forwarded-For") != "" {
		return false
	}
	return true
}

// requestContext holds the per-request auth state that handlers and
// templates need.
type requestContext struct {
	csrfToken     string
	authenticated bool
	local         bool
}

// requestContextKey identifies the requestContext stored on a request.
type requestContextKey struct{}

// serveWithContext hands the resolved session state down to the handler.
func (ws *WebServer) serveWithContext(w http.ResponseWriter, r *http.Request, ctx *requestContext, next http.Handler) {
	next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestContextKey{}, ctx)))
}

// contextFromRequest returns the session state the middleware attached. It
// yields a zero value for requests that bypassed the middleware, such as
// handlers called directly from tests.
func contextFromRequest(r *http.Request) *requestContext {
	if r == nil {
		return &requestContext{}
	}
	if ctx, ok := r.Context().Value(requestContextKey{}).(*requestContext); ok {
		return ctx
	}
	return &requestContext{}
}

// contextFor loads or creates the session for a request, issuing a cookie
// when needed. Anonymous visitors get a session too so the login form has a
// CSRF token.
func (ws *WebServer) contextFor(w http.ResponseWriter, r *http.Request) (*requestContext, error) {
	ctx := &requestContext{local: ws.isLocalRequest(r)}

	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		if sess, ok := ws.sessions.get(cookie.Value); ok {
			ws.sessions.touch(cookie.Value)
			ctx.csrfToken = sess.csrfToken
			ctx.authenticated = sess.authenticated
			return ctx, nil
		}
	}

	token, sess, err := ws.sessions.create(false)
	if err != nil {
		return nil, err
	}
	ws.setSessionCookie(w, token)
	ctx.csrfToken = sess.csrfToken
	ctx.authenticated = false
	return ctx, nil
}

// cookiePath scopes the session cookie to the mount point, so an admin UI
// at /admin does not send its cookie to the rest of the site.
func (ws *WebServer) cookiePath() string {
	if ws.config.BasePath == "" {
		return "/"
	}
	return ws.config.BasePath + "/"
}

func (ws *WebServer) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     ws.cookiePath(),
		HttpOnly: true,
		Secure:   ws.config.BehindProxy,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionLifetime.Seconds()),
	})
}

func (ws *WebServer) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     ws.cookiePath(),
		HttpOnly: true,
		Secure:   ws.config.BehindProxy,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// securityHeaders sets the response headers that limit the damage a
// cross-site or injection bug could do.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		// Cover art is fetched straight from the search APIs, so remote
		// https images are allowed; nothing else may load cross-origin.
		h.Set("Content-Security-Policy",
			"default-src 'self'; "+
				"img-src 'self' data: https:; "+
				"style-src 'self' 'unsafe-inline'; "+
				"script-src 'self' 'unsafe-inline'; "+
				"connect-src 'self'; "+
				"form-action 'self'; "+
				"frame-ancestors 'self'; "+
				"base-uri 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "SAMEORIGIN")
		h.Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

// noStore keeps browsers and any intermediate cache from retaining admin
// pages, which contain the whole collection and a CSRF token.
func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// authenticate is the gate in front of every route except /login. It
// establishes the session, enforces login, and validates the CSRF token on
// state-changing requests.
func (ws *WebServer) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, err := ws.contextFor(w, r)
		if err != nil {
			log.Printf("session error: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if ws.config.AuthEnabled() && !ctx.authenticated {
			if isStateChanging(r) {
				http.Error(w, "Not logged in", http.StatusUnauthorized)
				return
			}
			// r.URL is already stripped of the mount point by StripPrefix,
			// so "next" is an internal path and gets prefixed on the way out.
			http.Redirect(w, r, ws.config.URL("/login")+"?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
			return
		}

		if isStateChanging(r) && !ws.validCSRF(r, ctx.csrfToken) {
			http.Error(w, "Invalid or missing CSRF token. Reload the page and try again.", http.StatusForbidden)
			return
		}

		ws.serveWithContext(w, r, ctx, next)
	})
}

// isStateChanging reports whether a request may modify data and therefore
// needs a CSRF token.
func isStateChanging(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	return true
}

// validCSRF accepts the token from either a form field or, for the fetch()
// calls in the book form, a request header.
func (ws *WebServer) validCSRF(r *http.Request, expected string) bool {
	if expected == "" {
		return false
	}

	if header := r.Header.Get(csrfHeaderName); header != "" {
		return subtle.ConstantTimeCompare([]byte(header), []byte(expected)) == 1
	}

	// ParseForm consumes the body; JSON endpoints send the header instead,
	// so only form posts reach this branch.
	if err := r.ParseForm(); err != nil {
		return false
	}
	supplied := r.PostFormValue(csrfFormField)
	return subtle.ConstantTimeCompare([]byte(supplied), []byte(expected)) == 1
}

// requireLocal blocks the deploy and rollback routes for anyone who did not
// connect to this machine directly.
func (ws *WebServer) requireLocal(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !ws.isLocalRequest(r) {
			ws.renderErrorStatus(w, r, http.StatusForbidden, "Not available remotely",
				fmt.Errorf("deploying and rolling back run git commands on the server; run them from a browser on the machine itself"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (ws *WebServer) loginHandler(w http.ResponseWriter, r *http.Request) {
	ctx, err := ws.contextFor(w, r)
	if err != nil {
		log.Printf("session error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Nothing to log in to when no password is configured.
	if !ws.config.AuthEnabled() {
		http.Redirect(w, r, ws.config.URL("/"), http.StatusSeeOther)
		return
	}

	next := sanitizeNextPath(r.FormValue("next"))

	if r.Method != http.MethodPost {
		if ctx.authenticated {
			http.Redirect(w, r, ws.config.URL(next), http.StatusSeeOther)
			return
		}
		ws.renderLogin(w, http.StatusOK, ctx, next, "")
		return
	}

	if !ws.validCSRF(r, ctx.csrfToken) {
		ws.renderLogin(w, http.StatusForbidden, ctx, next, "Your login form expired. Try again.")
		return
	}

	addr := ws.clientIP(r)
	if wait := ws.loginThrottle.blockedFor(addr); wait > 0 {
		ws.renderLogin(w, http.StatusTooManyRequests, ctx, next,
			fmt.Sprintf("Too many failed attempts. Try again in %d minute(s).", int(wait.Minutes())+1))
		return
	}

	password := r.PostFormValue("password")
	if bcrypt.CompareHashAndPassword([]byte(ws.config.PasswordHash), []byte(password)) != nil {
		ws.loginThrottle.recordFailure(addr)
		log.Printf("failed admin login from %s", addr)
		ws.renderLogin(w, http.StatusUnauthorized, ctx, next, "Incorrect password.")
		return
	}

	ws.loginThrottle.recordSuccess(addr)

	// Issue a brand new session on login so a token an attacker planted
	// beforehand cannot become an authenticated one.
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		ws.sessions.delete(cookie.Value)
	}
	token, _, err := ws.sessions.create(true)
	if err != nil {
		log.Printf("session error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	ws.setSessionCookie(w, token)
	log.Printf("admin login from %s", addr)

	http.Redirect(w, r, ws.config.URL(next), http.StatusSeeOther)
}

func (ws *WebServer) logoutHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, ws.config.URL("/"), http.StatusSeeOther)
		return
	}
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		ws.sessions.delete(cookie.Value)
	}
	ws.clearSessionCookie(w)
	http.Redirect(w, r, ws.config.URL("/login"), http.StatusSeeOther)
}

func (ws *WebServer) renderLogin(w http.ResponseWriter, status int, ctx *requestContext, next, errMsg string) {
	data := PageData{
		Title:     "Sign In",
		HideNav:   true,
		CSRFToken: ctx.csrfToken,
		NextPath:  next,
		Error:     errMsg,
	}
	ws.writeTemplate(w, status, "login", data)
}

// sanitizeNextPath keeps post-login redirects on this site. Anything that
// could point at another origin falls back to the home page.
func sanitizeNextPath(next string) string {
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		return "/"
	}
	if strings.Contains(next, "\\") || strings.Contains(next, "\n") || strings.Contains(next, "\r") {
		return "/"
	}
	if next == "/login" || strings.HasPrefix(next, "/login?") {
		return "/"
	}
	return next
}

// startSessionJanitor periodically clears expired sessions and stale login
// failure records.
func (ws *WebServer) startSessionJanitor() {
	ticker := time.NewTicker(10 * time.Minute)
	go func() {
		for range ticker.C {
			ws.sessions.sweep()
			ws.loginThrottle.sweep()
		}
	}()
}
