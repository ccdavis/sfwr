package web

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

const testPassword = "correct horse battery staple"

func testConfig(t *testing.T, withPassword bool) Config {
	t.Helper()
	cfg := Config{BindAddr: "127.0.0.1", Port: "8080"}
	if withPassword {
		hash, err := HashPassword(testPassword)
		if err != nil {
			t.Fatalf("HashPassword failed: %v", err)
		}
		cfg.PasswordHash = hash
	}
	return cfg
}

// setupAuthServer builds a server with just enough template machinery for
// the middleware to render its refusals, so tests assert on real status
// codes rather than on the 500 a missing template would produce.
func setupAuthServer(t *testing.T, cfg Config) *WebServer {
	t.Helper()

	errorTmpl := template.Must(template.New("error.html").Parse(`<h1>Error</h1><p>{{.Error}}</p>`))

	ws := &WebServer{
		db:            setupTestDB(),
		imageDir:      "test_images",
		config:        cfg,
		sessions:      newSessionStore(),
		loginThrottle: newLoginThrottle(),
		templates:     map[string]*template.Template{"error": errorTmpl},
	}
	return ws
}

func TestConfigValidateRejectsRemoteWithoutPassword(t *testing.T) {
	cfg := Config{BindAddr: "0.0.0.0", Port: "8080"}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected a non-loopback bind with no password to be rejected")
	}
	if !strings.Contains(err.Error(), PasswordHashEnvVar) {
		t.Errorf("error should name the password env var, got: %v", err)
	}
}

func TestConfigValidateRejectsRemotePlaintext(t *testing.T) {
	cfg := testConfig(t, true)
	cfg.BindAddr = "0.0.0.0"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected a non-loopback bind without -behind-proxy to be rejected")
	}

	cfg.BehindProxy = true
	if err := cfg.Validate(); err != nil {
		t.Errorf("password + behind-proxy should be accepted, got: %v", err)
	}
}

func TestConfigValidateAllowsLoopbackWithoutPassword(t *testing.T) {
	cfg := Config{BindAddr: "127.0.0.1", Port: "8080"}
	if err := cfg.Validate(); err != nil {
		t.Errorf("loopback without a password should be allowed, got: %v", err)
	}
}

func TestIsLoopbackAddr(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1": true,
		"localhost": true,
		"::1":       true,
		"0.0.0.0":   false,
		"":          false,
		"10.0.0.5":  false,
	}
	for addr, want := range cases {
		if got := isLoopbackAddr(addr); got != want {
			t.Errorf("isLoopbackAddr(%q) = %v, want %v", addr, got, want)
		}
	}
}

func TestHashPasswordRejectsShortPasswords(t *testing.T) {
	if _, err := HashPassword("short"); err == nil {
		t.Error("expected short passwords to be rejected")
	}
}

func TestUnauthenticatedGetRedirectsToLogin(t *testing.T) {
	ws := setupAuthServer(t, testConfig(t, true))
	handler := ws.authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not run for an unauthenticated request")
	}))

	req := httptest.NewRequest("GET", "/books?sort=title", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("got status %d, want %d", rr.Code, http.StatusSeeOther)
	}
	location := rr.Header().Get("Location")
	if !strings.HasPrefix(location, "/login?next=") {
		t.Errorf("expected a redirect to /login, got %q", location)
	}
	if !strings.Contains(location, url.QueryEscape("/books?sort=title")) {
		t.Errorf("redirect should preserve the requested page, got %q", location)
	}
}

func TestUnauthenticatedPostIsRejected(t *testing.T) {
	ws := setupAuthServer(t, testConfig(t, true))
	handler := ws.authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not run for an unauthenticated POST")
	}))

	req := httptest.NewRequest("POST", "/books/delete/1", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestNoAuthConfiguredLetsRequestsThrough(t *testing.T) {
	ws := setupAuthServer(t, testConfig(t, false))
	reached := false
	handler := ws.authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
	}))

	req := httptest.NewRequest("GET", "/books", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !reached {
		t.Error("with no password configured, requests should be served")
	}
}

// authenticatedRequest returns a request carrying a logged-in session, plus
// that session's CSRF token.
func authenticatedRequest(t *testing.T, ws *WebServer, method, path string) (*http.Request, string) {
	t.Helper()
	token, sess, err := ws.sessions.create(true)
	if err != nil {
		t.Fatalf("could not create session: %v", err)
	}
	req := httptest.NewRequest(method, path, nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	return req, sess.csrfToken
}

func TestPostWithoutCSRFTokenIsRejected(t *testing.T) {
	ws := setupAuthServer(t, testConfig(t, true))
	handler := ws.authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not run without a CSRF token")
	}))

	req, _ := authenticatedRequest(t, ws, "POST", "/books/delete/1")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusForbidden)
	}
}

func TestPostWithWrongCSRFTokenIsRejected(t *testing.T) {
	ws := setupAuthServer(t, testConfig(t, true))
	handler := ws.authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not run with a bad CSRF token")
	}))

	req, _ := authenticatedRequest(t, ws, "POST", "/books/delete/1")
	req.Header.Set(csrfHeaderName, "not-the-right-token")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusForbidden)
	}
}

func TestPostWithCSRFHeaderSucceeds(t *testing.T) {
	ws := setupAuthServer(t, testConfig(t, true))
	reached := false
	handler := ws.authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
	}))

	req, csrf := authenticatedRequest(t, ws, "POST", "/books/delete/1")
	req.Header.Set(csrfHeaderName, csrf)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !reached {
		t.Errorf("expected the handler to run, got status %d", rr.Code)
	}
}

func TestPostWithCSRFFormFieldSucceeds(t *testing.T) {
	ws := setupAuthServer(t, testConfig(t, true))
	reached := false
	handler := ws.authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
	}))

	token, sess, err := ws.sessions.create(true)
	if err != nil {
		t.Fatalf("could not create session: %v", err)
	}
	form := url.Values{csrfFormField: {sess.csrfToken}}
	req := httptest.NewRequest("POST", "/books/delete/1", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !reached {
		t.Errorf("expected the handler to run, got status %d", rr.Code)
	}
}

// A CSRF token belonging to a different session must not be accepted, which
// is what makes the double-submit check meaningful.
func TestCSRFTokenFromAnotherSessionIsRejected(t *testing.T) {
	ws := setupAuthServer(t, testConfig(t, true))
	handler := ws.authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not run with another session's token")
	}))

	_, otherSess, err := ws.sessions.create(true)
	if err != nil {
		t.Fatalf("could not create session: %v", err)
	}

	req, _ := authenticatedRequest(t, ws, "POST", "/books/delete/1")
	req.Header.Set(csrfHeaderName, otherSess.csrfToken)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusForbidden)
	}
}

func TestGetDoesNotRequireCSRF(t *testing.T) {
	ws := setupAuthServer(t, testConfig(t, true))
	reached := false
	handler := ws.authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
	}))

	req, _ := authenticatedRequest(t, ws, "GET", "/books")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !reached {
		t.Errorf("GET should not need a CSRF token, got status %d", rr.Code)
	}
}

func TestSessionExpiryIsEnforced(t *testing.T) {
	store := newSessionStore()
	token, sess, err := store.create(true)
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := store.get(token); !ok {
		t.Fatal("a fresh session should be retrievable")
	}

	sess.expires = sess.expires.Add(-2 * sessionLifetime)
	if _, ok := store.get(token); ok {
		t.Error("an expired session should not be retrievable")
	}
}

func TestLoginThrottleLocksOutAfterRepeatedFailures(t *testing.T) {
	throttle := newLoginThrottle()
	addr := "203.0.113.7"

	for i := 0; i < maxLoginFailures-1; i++ {
		throttle.recordFailure(addr)
		if wait := throttle.blockedFor(addr); wait > 0 {
			t.Fatalf("locked out after %d failures, expected %d", i+1, maxLoginFailures)
		}
	}

	throttle.recordFailure(addr)
	if throttle.blockedFor(addr) <= 0 {
		t.Fatalf("expected a lockout after %d failures", maxLoginFailures)
	}

	// A different address must be unaffected.
	if throttle.blockedFor("198.51.100.4") > 0 {
		t.Error("one address's failures should not lock out another")
	}
}

func TestLoginThrottleClearsOnSuccess(t *testing.T) {
	throttle := newLoginThrottle()
	addr := "203.0.113.7"
	for i := 0; i < maxLoginFailures; i++ {
		throttle.recordFailure(addr)
	}
	throttle.recordSuccess(addr)
	if throttle.blockedFor(addr) > 0 {
		t.Error("a successful login should clear the lockout")
	}
}

func TestIsLocalRequest(t *testing.T) {
	direct := setupAuthServer(t, testConfig(t, true))
	proxied := setupAuthServer(t, testConfig(t, true))
	proxied.config.BehindProxy = true

	loopback := httptest.NewRequest("POST", "/deploy", nil)
	loopback.RemoteAddr = "127.0.0.1:54321"
	if !direct.isLocalRequest(loopback) {
		t.Error("a loopback connection should count as local")
	}

	remote := httptest.NewRequest("POST", "/deploy", nil)
	remote.RemoteAddr = "203.0.113.9:54321"
	if direct.isLocalRequest(remote) {
		t.Error("a remote connection should not count as local")
	}

	// Behind a proxy every connection arrives from loopback, so a forwarded
	// request must not be treated as local no matter what the client claims.
	forged := httptest.NewRequest("POST", "/deploy", nil)
	forged.RemoteAddr = "127.0.0.1:54321"
	forged.Header.Set("X-Forwarded-For", "127.0.0.1")
	if proxied.isLocalRequest(forged) {
		t.Error("a forwarded request must not gain local privileges by spoofing X-Forwarded-For")
	}
}

func TestRequireLocalBlocksRemoteDeploy(t *testing.T) {
	ws := setupAuthServer(t, testConfig(t, true))
	handler := ws.requireLocal(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("deploy must not run for a remote request")
	}))

	req := httptest.NewRequest("POST", "/deploy", nil)
	req.RemoteAddr = "203.0.113.9:54321"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusForbidden)
	}
}

func TestRequireLocalAllowsLoopbackDeploy(t *testing.T) {
	ws := setupAuthServer(t, testConfig(t, true))
	reached := false
	handler := ws.requireLocal(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
	}))

	req := httptest.NewRequest("POST", "/deploy", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !reached {
		t.Errorf("a loopback deploy should be allowed, got status %d", rr.Code)
	}
}

// Behind a proxy the throttle must key on the address the proxy reported,
// taking the last X-Forwarded-For entry so an attacker cannot dodge the
// lockout by prepending fake addresses.
func TestClientIPBehindProxy(t *testing.T) {
	ws := setupAuthServer(t, testConfig(t, true))
	ws.config.BehindProxy = true

	req := httptest.NewRequest("POST", "/login", nil)
	req.RemoteAddr = "127.0.0.1:9999"
	req.Header.Set("X-Forwarded-For", "10.0.0.1, 203.0.113.9")

	if got := ws.clientIP(req); got != "203.0.113.9" {
		t.Errorf("clientIP = %q, want the address the proxy appended", got)
	}
}

func TestClientIPIgnoresForwardedHeaderWhenNotProxied(t *testing.T) {
	ws := setupAuthServer(t, testConfig(t, true))

	req := httptest.NewRequest("POST", "/login", nil)
	req.RemoteAddr = "203.0.113.9:9999"
	req.Header.Set("X-Forwarded-For", "127.0.0.1")

	if got := ws.clientIP(req); got != "203.0.113.9" {
		t.Errorf("clientIP = %q, want the socket address", got)
	}
}

func TestSanitizeNextPath(t *testing.T) {
	cases := map[string]string{
		"/books?sort=title":     "/books?sort=title",
		"":                      "/",
		"//evil.example.com":    "/",
		"https://evil.example":  "/",
		"/login":                "/",
		"/login?next=/books":    "/",
		"/books\\@evil.example": "/",
	}
	for input, want := range cases {
		if got := sanitizeNextPath(input); got != want {
			t.Errorf("sanitizeNextPath(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSecurityHeadersAreSet(t *testing.T) {
	handler := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	for _, header := range []string{"Content-Security-Policy", "X-Content-Type-Options", "X-Frame-Options", "Referrer-Policy"} {
		if rr.Header().Get(header) == "" {
			t.Errorf("missing %s header", header)
		}
	}
}

func TestSessionCookieFlags(t *testing.T) {
	ws := setupAuthServer(t, testConfig(t, true))

	rr := httptest.NewRecorder()
	ws.setSessionCookie(rr, "token")
	cookie := rr.Result().Cookies()[0]

	if !cookie.HttpOnly {
		t.Error("the session cookie must be HttpOnly")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Error("the session cookie must be SameSite=Lax")
	}
	if cookie.Secure {
		t.Error("Secure should be off when not behind a TLS proxy")
	}

	ws.config.BehindProxy = true
	rr = httptest.NewRecorder()
	ws.setSessionCookie(rr, "token")
	if !rr.Result().Cookies()[0].Secure {
		t.Error("Secure must be set when behind a TLS proxy")
	}
}

// --- Base path (mounting the UI under a sub-path) ---

func basePathServer(t *testing.T, base string) *WebServer {
	t.Helper()
	cfg := testConfig(t, false)
	cfg.BasePath = base
	return setupAuthServer(t, cfg)
}

func TestConfigURLPrefixesPaths(t *testing.T) {
	root := Config{}
	if got := root.URL("/books"); got != "/books" {
		t.Errorf("with no base path, URL(/books) = %q, want /books", got)
	}
	if got := root.URL("/"); got != "/" {
		t.Errorf("with no base path, URL(/) = %q, want /", got)
	}

	mounted := Config{BasePath: "/admin"}
	if got := mounted.URL("/books"); got != "/admin/books" {
		t.Errorf("URL(/books) = %q, want /admin/books", got)
	}
	if got := mounted.URL("/"); got != "/admin/" {
		t.Errorf("URL(/) = %q, want /admin/", got)
	}
	if got := mounted.URL("/books?sort=title"); got != "/admin/books?sort=title" {
		t.Errorf("URL with a query = %q, want the query preserved", got)
	}
}

// The mount strips its prefix so handlers and route patterns keep seeing
// plain paths.
func TestMountStripsThePrefix(t *testing.T) {
	ws := basePathServer(t, "/admin")

	var seen string
	handler := ws.mount(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.URL.Path
	}))

	req := httptest.NewRequest("GET", "/admin/books?sort=title", nil)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if seen != "/books" {
		t.Errorf("handler saw %q, want /books", seen)
	}
}

func TestMountRedirectsBareBasePath(t *testing.T) {
	ws := basePathServer(t, "/admin")
	handler := ws.mount(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	req := httptest.NewRequest("GET", "/admin", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusMovedPermanently {
		t.Fatalf("got status %d, want %d", rr.Code, http.StatusMovedPermanently)
	}
	if loc := rr.Header().Get("Location"); loc != "/admin/" {
		t.Errorf("Location = %q, want /admin/", loc)
	}
}

// A request that misses the mount must 404 rather than being served, which
// would hide a misconfigured proxy.
func TestMountRejectsPathsOutsideIt(t *testing.T) {
	ws := basePathServer(t, "/admin")
	handler := ws.mount(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler must not run for a path outside the mount")
	}))

	for _, path := range []string{"/books", "/", "/adminx/books"} {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, httptest.NewRequest("GET", path, nil))
		if rr.Code != http.StatusNotFound {
			t.Errorf("%s: got status %d, want 404", path, rr.Code)
		}
	}
}

func TestMountIsANoOpWithoutABasePath(t *testing.T) {
	ws := basePathServer(t, "")

	var seen string
	handler := ws.mount(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.URL.Path
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/books", nil))

	if seen != "/books" {
		t.Errorf("handler saw %q, want /books", seen)
	}
}

// The login redirect has to point back inside the mount, or an
// unauthenticated visitor is bounced out of the application entirely.
func TestLoginRedirectStaysInsideTheMount(t *testing.T) {
	cfg := testConfig(t, true)
	cfg.BasePath = "/admin"
	ws := setupAuthServer(t, cfg)

	handler := ws.mount(ws.authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not run for an unauthenticated request")
	})))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest("GET", "/admin/books?sort=title", nil))

	location := rr.Header().Get("Location")
	if !strings.HasPrefix(location, "/admin/login?next=") {
		t.Fatalf("Location = %q, want a redirect to /admin/login", location)
	}
	// "next" is an internal path; it gets the prefix applied when used.
	if !strings.Contains(location, url.QueryEscape("/books?sort=title")) {
		t.Errorf("Location = %q, want the original page preserved in next", location)
	}
}

// The session cookie must not be sent to the rest of the site that shares
// the domain.
func TestSessionCookieIsScopedToTheMount(t *testing.T) {
	ws := basePathServer(t, "/admin")

	rr := httptest.NewRecorder()
	ws.setSessionCookie(rr, "token")
	if got := rr.Result().Cookies()[0].Path; got != "/admin/" {
		t.Errorf("cookie Path = %q, want /admin/", got)
	}

	root := basePathServer(t, "")
	rr = httptest.NewRecorder()
	root.setSessionCookie(rr, "token")
	if got := rr.Result().Cookies()[0].Path; got != "/" {
		t.Errorf("cookie Path = %q, want /", got)
	}
}

// A loopback bind is not a protection when a reverse proxy fronts the
// server: the proxy connects over loopback, so an unauthenticated server
// would be published to the internet. Starting `sfwr` with no password in
// the environment must fail rather than quietly expose the UI.
func TestConfigValidateRejectsProxiedWithoutPassword(t *testing.T) {
	cfg := Config{BindAddr: "127.0.0.1", Port: "8080", BehindProxy: true}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("a proxied server with no password must be refused")
	}
	if !strings.Contains(err.Error(), PasswordHashEnvVar) {
		t.Errorf("the error should say how to fix it, got: %v", err)
	}

	withPassword := testConfig(t, true)
	withPassword.BehindProxy = true
	if err := withPassword.Validate(); err != nil {
		t.Errorf("a proxied server with a password should start, got: %v", err)
	}
}
