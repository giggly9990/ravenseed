package api

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func TestTrustedProxyClientAddress(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("::1/128")}
	for _, test := range []struct{ peer, forwarded, want string }{
		{"192.0.2.1:80", "198.51.100.1", "192.0.2.1"},
		{"10.0.0.1:80", "198.51.100.1", "198.51.100.1"},
		{"10.0.0.1:80", "1.1.1.1, 198.51.100.1, 10.0.0.2", "198.51.100.1"},
		{"10.0.0.1:80", "garbage, 198.51.100.1", "198.51.100.1"},
		{"10.0.0.1:80", "198.51.100.1, garbage", "10.0.0.1"},
		{"10.0.0.1:80", "198.51.100.1, garbage, 10.0.0.2", "10.0.0.2"},
		{"10.0.0.1:80", strings.Repeat("192.0.2.9, ", 40) + "198.51.100.1", "198.51.100.1"},
		{"[::1]:80", "2001:db8::1", "2001:db8::1"},
		{"10.0.0.1:80", "", "10.0.0.1"},
	} {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = test.peer
		r.Header.Set("X-Forwarded-For", test.forwarded)
		if got := clientAddress(r, trusted); got != test.want {
			t.Errorf("%s %q: got %s want %s", test.peer, test.forwarded, got, test.want)
		}
	}
}

func TestRateLimitSeparatesTrustedClients(t *testing.T) {
	h := rateLimit(netip.MustParsePrefix("10.0.0.0/8"))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	request := func(peer, client string) int {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = peer
		r.Header.Set("X-Forwarded-For", client)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	for i := 0; i < 240; i++ {
		if request("10.0.0.1:80", "192.0.2.1") != 204 {
			t.Fatal("limited too early")
		}
	}
	if request("10.0.0.1:80", "192.0.2.1") != 429 || request("10.0.0.1:80", "192.0.2.2") != 204 {
		t.Fatal("proxy clients share a quota")
	}
	// A client padding the header cannot fall into the proxy's own bucket.
	if request("10.0.0.1:80", "garbage, 192.0.2.1") != 429 || request("10.0.0.1:80", strings.Repeat("192.0.2.5,", 40)+"192.0.2.1") != 429 {
		t.Fatal("malformed forwarding escaped the client's quota")
	}
	for i := 0; i < 240; i++ {
		request("198.51.100.1:80", "192.0.2.3")
	}
	if request("198.51.100.1:80", "192.0.2.4") != 429 {
		t.Fatal("untrusted forwarding bypassed limit")
	}
}
