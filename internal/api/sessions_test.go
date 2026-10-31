package api

import (
	"context"
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"ravenseed/internal/auth"
	"strings"
	"sync"
	"testing"
	"time"
)

const browserPassword = "test-admin-password-123"

var browserHash = sync.OnceValue(func() string {
	hash, err := auth.HashPassword(browserPassword)
	if err != nil {
		panic(err)
	}
	return hash
})

func browserFixture(t *testing.T) (*fixture, http.Handler, string) {
	t.Helper()
	f := newFixture(t, true)
	f.service.Config.WebUsername, f.service.Config.WebPasswordHash = "admin", browserHash()
	f.service.Config.WebCookieSecure = true
	web := t.TempDir()
	for path, content := range map[string]string{"index.html": "private app shell", "app.js": "private asset", "docs.html": "private docs"} {
		if err := os.WriteFile(filepath.Join(web, path), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return f, New(f.service, web), web
}

func browserRequest(handler http.Handler, method, path string, body url.Values, cookie *http.Cookie, origin string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "https://raven.example"+path, strings.NewReader(body.Encode()))
	if body != nil {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func signIn(t *testing.T, handler http.Handler) *http.Cookie {
	t.Helper()
	w := browserRequest(handler, "POST", "/login", url.Values{"username": {"admin"}, "password": {browserPassword}, "next": {"/history"}}, nil, "https://raven.example")
	if w.Code != 303 || w.Header().Get("Location") != "/history" {
		t.Fatalf("login: %d %s", w.Code, w.Body.String())
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal("missing session cookie")
	}
	return cookies[0]
}

func TestServerProtectsEntireWebUI(t *testing.T) {
	f, handler, _ := browserFixture(t)
	for _, path := range []string{"/", "/history", "/settings", "/uploads", "/app.js", "/index.html", "/docs.html"} {
		w := browserRequest(handler, "GET", path, nil, nil, "")
		if w.Code != 303 || !strings.HasPrefix(w.Header().Get("Location"), "/login?") || strings.Contains(w.Body.String(), "private") {
			t.Fatalf("public UI %s: %d", path, w.Code)
		}
	}
	for _, path := range []string{"/api/docs", "/api/openapi.json", "/api/v1/settings", "/api/v1/session", "/api/v1/ready", "/api/v1/uploads"} {
		if w := browserRequest(handler, "GET", path, nil, nil, ""); w.Code != 401 {
			t.Fatalf("public API %s: %d", path, w.Code)
		}
	}
	for _, path := range []string{"/health", "/api/v1/health", "/login"} {
		if w := browserRequest(handler, "GET", path, nil, nil, ""); w.Code != 200 {
			t.Fatalf("public endpoint %s: %d", path, w.Code)
		}
	}
	if w := browserRequest(handler, "GET", "/login", nil, nil, ""); w.Header().Get("Referrer-Policy") != "same-origin" {
		t.Fatal("login page must preserve the origin on same-origin form submissions")
	}
	for _, path := range []string{"/api/v1/settings", "/app.js"} {
		r := httptest.NewRequest("GET", "https://raven.example"+path, nil)
		r.Header.Set("Authorization", "Bearer "+f.service.Config.APIToken)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		want := 200
		if path == "/app.js" {
			want = 303
		}
		if w.Code != want {
			t.Fatalf("bearer scope %s: %d", path, w.Code)
		}
	}
}

func TestSessionLoginLogoutAndCSRF(t *testing.T) {
	f, handler, web := browserFixture(t)
	cookie := signIn(t, handler)
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/" || cookie.Domain != "" || cookie.Name != "__Host-ravenseed_session" || cookie.MaxAge != 43200 || len(cookie.Value) != 43 {
		t.Fatalf("unsafe cookie: %+v", cookie)
	}
	for _, path := range []string{"/", "/app.js", "/api/docs", "/api/v1/settings", "/api/v1/session"} {
		w := browserRequest(handler, "GET", path, nil, cookie, "")
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("session access %s: %d", path, w.Code)
		}
		if w.Header().Get("Referrer-Policy") != "same-origin" {
			t.Fatalf("session page %s must preserve the origin on logout", path)
		}
	}
	for _, origin := range []string{"", "null", "https://attacker.example", "http://raven.example"} {
		if w := browserRequest(handler, "POST", "/logout", url.Values{}, cookie, origin); w.Code != 403 {
			t.Fatalf("logout CSRF accepted: %q %d", origin, w.Code)
		}
		if w := browserRequest(handler, "PUT", "/api/v1/settings", url.Values{}, cookie, origin); w.Code != 403 {
			t.Fatalf("API CSRF accepted: %q %d", origin, w.Code)
		}
	}
	r := httptest.NewRequest("PUT", "https://raven.example/api/v1/settings/torrents", strings.NewReader(`{"defaultInstanceId":0,"categories":[]}`))
	r.Header.Set("Origin", "https://raven.example")
	r.Header.Set("Content-Type", "application/json")
	r.AddCookie(cookie)
	accepted := httptest.NewRecorder()
	handler.ServeHTTP(accepted, r)
	if accepted.Code != 200 {
		t.Fatalf("same-origin browser mutation rejected: %d %s", accepted.Code, accepted.Body.String())
	}
	w := browserRequest(handler, "POST", "/logout", url.Values{}, cookie, "https://raven.example")
	if w.Code != 303 || w.Result().Cookies()[0].MaxAge != -1 {
		t.Fatal("logout did not clear cookie")
	}
	if w := browserRequest(handler, "GET", "/api/v1/settings", nil, cookie, ""); w.Code != 401 {
		t.Fatal("revoked cookie accepted")
	}
	cookie = signIn(t, handler)
	restarted := New(f.service, web)
	if w := browserRequest(restarted, "GET", "/api/v1/settings", nil, cookie, ""); w.Code != 401 {
		t.Fatal("session survived restart")
	}
}

func TestLoginValidationAndThrottle(t *testing.T) {
	_, handler, _ := browserFixture(t)
	form := url.Values{"username": {"admin"}, "password": {browserPassword}}
	for _, origin := range []string{"", "null", "https://attacker.example", "http://raven.example"} {
		if w := browserRequest(handler, "POST", "/login", form, nil, origin); w.Code != 403 {
			t.Fatalf("login CSRF accepted: %q %d", origin, w.Code)
		}
	}
	form.Set("password", "incorrect-password")
	for i := 0; i < 5; i++ {
		if w := browserRequest(handler, "POST", "/login", form, nil, "https://raven.example"); w.Code != 401 || len(w.Result().Cookies()) != 0 {
			t.Fatalf("invalid login %d", w.Code)
		}
	}
	if w := browserRequest(handler, "POST", "/login", form, nil, "https://raven.example"); w.Code != 429 {
		t.Fatal("login not throttled")
	}
	for _, value := range []string{"https://attacker.example", "//attacker.example", "/%2fattack", "/\\attack", "/%09/attack", "///attacker.example", "/.//attacker.example", "/a/../login", "/login", "/api/v1/settings"} {
		if safeNext(value) != "/" {
			t.Fatalf("unsafe redirect accepted: %q", value)
		}
	}
}

func TestSessionExpiryAndRevocationCancelRequests(t *testing.T) {
	sessions := newBrowserSessions()
	token, err := sessions.create()
	if err != nil {
		t.Fatal(err)
	}
	session := sessions.find(token)
	if session == nil {
		t.Fatal("session missing")
	}
	sessions.revoke(token)
	if session.ctx.Err() != context.Canceled || sessions.find(token) != nil {
		t.Fatal("revocation did not cancel session")
	}
	token, err = sessions.create()
	if err != nil {
		t.Fatal(err)
	}
	sessions.mu.Lock()
	sessions.entries[sha256.Sum256([]byte(token))].expires = time.Now().Add(-time.Second)
	sessions.mu.Unlock()
	if sessions.find(token) != nil {
		t.Fatal("expired session accepted")
	}
}

func TestLoginRequiresHTTPSOrTrustedProxy(t *testing.T) {
	f, handler, _ := browserFixture(t)
	request := func(trusted bool) *httptest.ResponseRecorder {
		if trusted {
			f.service.Config.TrustedProxies = []netip.Prefix{netip.MustParsePrefix("192.0.2.1/32")}
		}
		r := httptest.NewRequest("GET", "http://raven.example/login", nil)
		r.Header.Set("X-Forwarded-Proto", "https")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if w := request(false); strings.Contains(w.Body.String(), `name="password"`) {
		t.Fatal("spoofed proxy enabled HTTP password form")
	}
	if w := request(true); !strings.Contains(w.Body.String(), `name="password"`) {
		t.Fatal("trusted TLS proxy rejected")
	}
}

func TestLogoutWithExpiredSessionClearsCookie(t *testing.T) {
	_, handler, _ := browserFixture(t)
	stale := &http.Cookie{Name: "__Host-ravenseed_session", Value: strings.Repeat("a", 43)}
	w := browserRequest(handler, "POST", "/logout", url.Values{}, stale, "https://raven.example")
	if w.Code != 303 || w.Header().Get("Location") != "/login" || len(w.Result().Cookies()) != 1 || w.Result().Cookies()[0].MaxAge != -1 {
		t.Fatalf("stale logout: %d %v", w.Code, w.Result().Cookies())
	}
	if w := browserRequest(handler, "POST", "/logout", url.Values{}, stale, "https://attacker.example"); w.Code != 403 {
		t.Fatalf("cross-site logout accepted: %d", w.Code)
	}
}
