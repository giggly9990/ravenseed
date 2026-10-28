package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"log/slog"
	"math"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"ravenseed/internal/auth"
	"ravenseed/internal/config"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

const sessionLifetime = 12 * time.Hour

type webSession struct {
	expires time.Time
	ctx     context.Context
	cancel  context.CancelFunc
}
type browserSessions struct {
	mu      sync.Mutex
	entries map[[32]byte]*webSession
	logins  *loginLimiter
	hashing chan struct{}
}

func newBrowserSessions() *browserSessions {
	return &browserSessions{entries: make(map[[32]byte]*webSession), logins: newLoginLimiter(), hashing: make(chan struct{}, 2)}
}

func (s *browserSessions) find(token string) *webSession {
	if len(token) != 43 {
		return nil
	}
	key := sha256.Sum256([]byte(token))
	s.mu.Lock()
	defer s.mu.Unlock()
	session := s.entries[key]
	if session != nil && (!time.Now().Before(session.expires) || session.ctx.Err() != nil) {
		session.cancel()
		delete(s.entries, key)
		return nil
	}
	return session
}

func (s *browserSessions) revoke(token string) {
	key := sha256.Sum256([]byte(token))
	s.mu.Lock()
	defer s.mu.Unlock()
	if session := s.entries[key]; session != nil {
		session.cancel()
		delete(s.entries, key)
	}
}

func (s *browserSessions) create() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	expires := time.Now().Add(sessionLifetime)
	ctx, cancel := context.WithDeadline(context.Background(), expires)
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, session := range s.entries {
		if !time.Now().Before(session.expires) {
			session.cancel()
			delete(s.entries, key)
		}
	}
	if len(s.entries) >= 128 {
		var oldest [32]byte
		first := true
		for key, session := range s.entries {
			if first || session.expires.Before(s.entries[oldest].expires) {
				oldest = key
				first = false
			}
		}
		s.entries[oldest].cancel()
		delete(s.entries, oldest)
	}
	s.entries[sha256.Sum256([]byte(token))] = &webSession{expires: expires, ctx: ctx, cancel: cancel}
	return token, nil
}

func (a *API) cookieName() string {
	if a.Service.Config.WebCookieSecure {
		return "__Host-ravenseed_session"
	}
	return "ravenseed_session"
}

func (a *API) session(r *http.Request) *webSession {
	cookie, err := r.Cookie(a.cookieName())
	if err != nil {
		return nil
	}
	return a.sessions.find(cookie.Value)
}

// fromTrustedProxy reports a direct peer listed in TRUSTED_PROXIES.
func (a *API) fromTrustedProxy(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	peer, err := netip.ParseAddr(host)
	return err == nil && trusted(peer.Unmap(), a.Service.Config.TrustedProxies)
}

func (a *API) secureRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return a.fromTrustedProxy(r) && r.Header.Get("X-Forwarded-Proto") == "https"
}

// requestHost is the host the browser addressed: a trusted proxy's
// X-Forwarded-Host (its own, last entry) when set, else the Host header.
func (a *API) requestHost(r *http.Request) string {
	if forwarded := r.Header.Values("X-Forwarded-Host"); len(forwarded) > 0 && a.fromTrustedProxy(r) {
		hosts := strings.Split(forwarded[len(forwarded)-1], ",")
		if host := strings.TrimSpace(hosts[len(hosts)-1]); host != "" {
			return host
		}
	}
	return r.Host
}

// allowedHost guards installs without authentication against DNS rebinding:
// a page on another site whose name now resolves to this server still sends
// that name. Loopback names and ALLOWED_HOSTS are accepted.
func (a *API) allowedHost(r *http.Request) bool {
	if a.Service.Config.AuthRequired() {
		return true
	}
	host := a.requestHost(r)
	return config.IsLoopbackHost(host) || slices.Contains(a.Service.Config.AllowedHosts, config.NormalizeHost(host))
}

func (a *API) sameOrigin(r *http.Request) bool {
	origin, err := url.Parse(r.Header.Get("Origin"))
	scheme := "http"
	if a.secureRequest(r) {
		scheme = "https"
	}
	return err == nil && origin.Scheme == scheme && strings.EqualFold(origin.Host, a.requestHost(r)) && origin.User == nil && origin.Path == "" && origin.RawQuery == "" && origin.Fragment == ""
}

func (a *API) protect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.Service.Config.AuthRequired() {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		// Logout must also work with an expired session: it clears the cookie.
		if r.URL.Path == "/login" || r.URL.Path == "/logout" || r.URL.Path == "/health" || r.URL.Path == "/api/v1/health" {
			next.ServeHTTP(w, r)
			return
		}
		isAPI := strings.HasPrefix(r.URL.Path, "/api/")
		if isAPI && r.Header.Get("Authorization") != "" {
			if a.validBearer(r.Header.Get("Authorization")) {
				next.ServeHTTP(w, r)
				return
			}
		} else if session := a.session(r); session != nil {
			if r.Method != "GET" && r.Method != "HEAD" && !a.sameOrigin(r) {
				fail(w, 403, "origin_rejected", "A same-origin browser request is required")
				return
			}
			if strings.HasPrefix(r.URL.Path, "/assets/") {
				// Build assets have content-hashed names; only this browser keeps them.
				w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
			}
			ctx, cancel := context.WithCancel(r.Context())
			stop := context.AfterFunc(session.ctx, cancel)
			defer stop()
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		if isAPI {
			w.Header().Set("WWW-Authenticate", "Bearer")
			fail(w, 401, "unauthorized", "Sign in or provide a valid API bearer token")
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			fail(w, 401, "unauthorized", "Sign in to continue")
			return
		}
		http.Redirect(w, r, "/login?next="+url.QueryEscape(safeNext(r.URL.RequestURI())), http.StatusSeeOther)
	})
}

// validBearer compares digests, so neither the token's content nor its length
// shows in the timing. The scheme is case-insensitive.
func (a *API) validBearer(header string) bool {
	token := a.Service.Config.APIToken
	scheme, value, ok := strings.Cut(strings.TrimSpace(header), " ")
	if token == "" || !ok || !strings.EqualFold(scheme, "Bearer") {
		return false
	}
	got, want := sha256.Sum256([]byte(strings.TrimSpace(value))), sha256.Sum256([]byte(token))
	return subtle.ConstantTimeCompare(got[:], want[:]) == 1
}

func safeNext(value string) string {
	parsed, err := url.Parse(value)
	if err != nil || !strings.HasPrefix(value, "/") || parsed.Host != "" || parsed.IsAbs() || strings.HasPrefix(parsed.Path, "//") || strings.Contains(value+parsed.Path, "\\") || strings.ContainsFunc(value+parsed.Path, unicode.IsControl) || parsed.Path == "/login" || parsed.Path == "/logout" || strings.HasPrefix(parsed.Path, "/api/") {
		return "/"
	}
	return value
}

func (a *API) setSessionCookie(w http.ResponseWriter, token string, maxAge int) {
	expires := time.Now().Add(sessionLifetime)
	if maxAge < 0 {
		expires = time.Unix(1, 0)
	}
	http.SetCookie(w, &http.Cookie{Name: a.cookieName(), Value: token, Path: "/", HttpOnly: true, Secure: a.Service.Config.WebCookieSecure, SameSite: http.SameSiteStrictMode, MaxAge: maxAge, Expires: expires})
}

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !a.Service.Config.AuthRequired() {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	next := safeNext(r.URL.Query().Get("next"))
	if r.Method == "GET" {
		if a.session(r) != nil {
			http.Redirect(w, r, next, http.StatusSeeOther)
			return
		}
		a.loginPage(w, r, 200, "", next)
		return
	}
	if !a.sameOrigin(r) {
		a.loginPage(w, r, 403, "Sign in from this site's login page.", next)
		return
	}
	if a.Service.Config.WebPasswordHash == "" {
		a.loginPage(w, r, 503, "Browser login is not configured. Set WEB_USERNAME and a web password on the server.", next)
		return
	}
	if a.Service.Config.WebCookieSecure && !a.secureRequest(r) {
		a.loginPage(w, r, 403, "HTTPS is required to sign in.", next)
		return
	}
	client := clientAddress(r, a.Service.Config.TrustedProxies)
	limitKey := loginKey(client)
	if ok, wait := a.sessions.logins.allow(limitKey, time.Now()); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(max(1, int(math.Ceil(wait.Seconds())))))
		a.loginPage(w, r, 429, "Too many sign-in attempts. Try again later.", next)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	if mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mediaType != "application/x-www-form-urlencoded" || r.ParseForm() != nil {
		a.loginPage(w, r, 400, "Invalid sign-in form.", next)
		return
	}
	next = safeNext(r.PostForm.Get("next"))
	select {
	case a.sessions.hashing <- struct{}{}:
		defer func() { <-a.sessions.hashing }()
	default:
		a.loginPage(w, r, 429, "Sign-in is busy. Try again shortly.", next)
		return
	}
	validPassword := auth.CheckPassword(a.Service.Config.WebPasswordHash, r.PostForm.Get("password"))
	actualUser, expectedUser := sha256.Sum256([]byte(r.PostForm.Get("username"))), sha256.Sum256([]byte(a.Service.Config.WebUsername))
	if !validPassword || subtle.ConstantTimeCompare(actualUser[:], expectedUser[:]) != 1 {
		a.sessions.logins.fail(limitKey, time.Now())
		// One structured line per failure, for fail2ban and similar tools.
		slog.Warn("login failed", "client_ip", client)
		a.loginPage(w, r, 401, "Incorrect username or password.", next)
		return
	}
	a.sessions.logins.succeed(limitKey, time.Now())
	token, err := a.sessions.create()
	if err != nil {
		a.loginPage(w, r, 500, "Could not create a session.", next)
		return
	}
	if cookie, err := r.Cookie(a.cookieName()); err == nil {
		a.sessions.revoke(cookie.Value)
	}
	a.setSessionCookie(w, token, int(sessionLifetime.Seconds()))
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	if !a.sameOrigin(r) {
		fail(w, 403, "origin_rejected", "A same-origin browser request is required")
		return
	}
	if cookie, err := r.Cookie(a.cookieName()); err == nil {
		a.sessions.revoke(cookie.Value)
	}
	a.setSessionCookie(w, "", -1)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
