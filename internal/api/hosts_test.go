package api

import (
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"
)

func TestUnauthenticatedInstallAcceptsOnlyKnownHosts(t *testing.T) {
	f := newFixture(t, false)
	handler := New(f.service, t.TempDir())
	request := func(host, path, peer, forwardedHost string) int {
		r := httptest.NewRequest("GET", "http://placeholder"+path, nil)
		r.Host = host
		if peer != "" {
			r.RemoteAddr = peer
		}
		if forwardedHost != "" {
			r.Header.Set("X-Forwarded-Host", forwardedHost)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w.Code
	}
	for _, host := range []string{"localhost", "localhost:8088", "LOCALHOST.", "127.0.0.1:8080", "127.0.0.2", "[::1]:8088", "[::1]"} {
		if code := request(host, "/api/v1/settings", "", ""); code != 200 {
			t.Fatalf("loopback host %q rejected: %d", host, code)
		}
	}
	// A rebinding page keeps its own name in Host.
	for _, host := range []string{"attacker.example", "attacker.example:8088", "192.168.1.5:8088", "localhost.attacker.example", ""} {
		if code := request(host, "/api/v1/settings", "", ""); code != 403 {
			t.Fatalf("foreign host %q accepted: %d", host, code)
		}
		if code := request(host, "/", "", ""); code != 403 {
			t.Fatalf("foreign host %q reached the UI: %d", host, code)
		}
	}
	// The container healthcheck works whatever the Host.
	for _, path := range []string{"/health", "/api/v1/health"} {
		if code := request("attacker.example", path, "", ""); code != 200 {
			t.Fatalf("health %s rejected: %d", path, code)
		}
	}
	f.service.Config.AllowedHosts = []string{"nas.lan", "192.168.1.5"}
	for _, host := range []string{"nas.lan", "NAS.lan:8088", "192.168.1.5:8088"} {
		if code := request(host, "/api/v1/settings", "", ""); code != 200 {
			t.Fatalf("allowed host %q rejected: %d", host, code)
		}
	}
	// A trusted proxy's X-Forwarded-Host names the browser's host; anyone
	// else's is ignored.
	f.service.Config.TrustedProxies = []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	if code := request("nas.lan", "/api/v1/settings", "10.0.0.2:5000", "attacker.example"); code != 403 {
		t.Fatalf("forwarded rebinding host accepted: %d", code)
	}
	if code := request("ravenseed:8080", "/api/v1/settings", "10.0.0.2:5000", "nas.lan"); code != 200 {
		t.Fatalf("forwarded allowed host rejected: %d", code)
	}
	if code := request("nas.lan", "/api/v1/settings", "192.0.2.9:5000", "attacker.example"); code != 200 {
		t.Fatalf("untrusted X-Forwarded-Host used: %d", code)
	}
}

func TestAuthenticatedInstallIgnoresHost(t *testing.T) {
	f := newFixture(t, true)
	r := httptest.NewRequest("GET", "http://attacker.example/api/v1/settings", nil)
	r.Header.Set("Authorization", "bearer "+f.service.Config.APIToken)
	w := httptest.NewRecorder()
	New(f.service, t.TempDir()).ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("authenticated request with lowercase scheme: %d", w.Code)
	}
	for _, header := range []string{"Bearer " + f.service.Config.APIToken + "x", "Basic " + f.service.Config.APIToken, f.service.Config.APIToken} {
		r.Header.Set("Authorization", header)
		w = httptest.NewRecorder()
		New(f.service, t.TempDir()).ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatalf("invalid authorization %q accepted: %d", header, w.Code)
		}
	}
}

func TestLoginLimiter(t *testing.T) {
	now := time.Now()
	l := newLoginLimiter()
	for i := 0; i < loginAttemptsPerMinute; i++ {
		if ok, _ := l.allow("192.0.2.1", now); !ok {
			t.Fatal("limited too early")
		}
	}
	if ok, wait := l.allow("192.0.2.1", now); ok || wait <= 0 || wait > time.Minute {
		t.Fatalf("per-client limit missing: %v %v", ok, wait)
	}

	// IPv6 clients share their /64.
	if loginKey("2001:db8::1") != loginKey("2001:db8::ffff:1") || loginKey("2001:db8::1") == loginKey("2001:db8:0:1::1") || loginKey("::ffff:192.0.2.1") != "192.0.2.1" {
		t.Fatal("unexpected IPv6 grouping")
	}

	// Failures elsewhere fill the global cap, but not for a known client.
	l = newLoginLimiter()
	if ok, _ := l.allow("192.0.2.50", now); !ok {
		t.Fatal("owner rejected")
	}
	l.succeed("192.0.2.50", now)
	for i := 0; i < loginFailuresPerMinute; i++ {
		key := netip.AddrFrom4([4]byte{198, 51, 100, byte(i)}).String()
		if ok, _ := l.allow(key, now); !ok {
			t.Fatalf("attacker %d limited early", i)
		}
		l.fail(key, now)
	}
	if ok, _ := l.allow("203.0.113.7", now); ok {
		t.Fatal("global failure cap missing")
	}
	if ok, _ := l.allow("192.0.2.50", now); !ok {
		t.Fatal("known client locked out by others' failures")
	}

	// Repeated failures lock the client out for longer each time.
	l = newLoginLimiter()
	at := now
	for i := 0; i < loginAttemptsPerMinute; i++ {
		l.allow("192.0.2.9", at)
		l.fail("192.0.2.9", at)
	}
	at = at.Add(61 * time.Second)
	if ok, _ := l.allow("192.0.2.9", at); !ok {
		t.Fatal("first lockout should last one minute")
	}
	l.fail("192.0.2.9", at)
	if ok, wait := l.allow("192.0.2.9", at.Add(61*time.Second)); ok || wait < 58*time.Second {
		t.Fatalf("second lockout should last two minutes: %v %v", ok, wait)
	}
	l.succeed("192.0.2.9", at.Add(3*time.Minute))
	if ok, _ := l.allow("192.0.2.9", at.Add(3*time.Minute)); !ok {
		t.Fatal("success did not clear the lockout")
	}
}
