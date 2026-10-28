package api

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// maxForwardedHops bounds the X-Forwarded-For walk.
const maxForwardedHops = 32

func trusted(ip netip.Addr, prefixes []netip.Prefix) bool {
	for _, prefix := range prefixes {
		if prefix.Contains(ip) {
			return true
		}
	}
	return false
}

// clientAddress identifies the client for rate limits. Behind trusted proxies
// it walks X-Forwarded-For from the right, the entries those proxies wrote,
// and stops at the first untrusted hop: entries further left are whatever the
// client sent. An unparsable hop ends the walk at the last trusted one.
func clientAddress(r *http.Request, prefixes []netip.Prefix) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	peer = peer.Unmap()
	if !trusted(peer, prefixes) {
		return peer.String()
	}
	chain := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for i, hops := len(chain)-1, 0; i >= 0 && hops < maxForwardedHops; i, hops = i-1, hops+1 {
		ip, err := netip.ParseAddr(strings.TrimSpace(chain[i]))
		if err != nil {
			break
		}
		peer = ip.Unmap()
		if !trusted(peer, prefixes) {
			break
		}
	}
	return peer.String()
}
