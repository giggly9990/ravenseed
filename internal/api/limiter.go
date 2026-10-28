package api

import (
	"net/netip"
	"sync"
	"time"
)

const (
	loginAttemptsPerMinute = 5
	loginFailuresPerMinute = 30
	loginKnownClientFor    = 30 * 24 * time.Hour
	loginStrikeMemory      = 24 * time.Hour
	loginMaxLockout        = time.Hour
	loginMaxClients        = 10000
	loginMaxKnownClients   = 1000
)

// loginLimiter throttles password checks per client, an IPv4 address or an
// IPv6 /64: five attempts a minute, then lockouts that double with each
// further failure. A global cap on failures slows guessing spread over many
// addresses; clients that signed in during the last 30 days are exempt from
// it, so failures from elsewhere cannot lock the owner out.
type loginLimiter struct {
	mu       sync.Mutex
	window   time.Time
	failures int
	attempts map[string]int
	strikes  map[string]loginStrike
	known    map[string]time.Time
}

type loginStrike struct {
	count       int
	last, until time.Time
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{attempts: map[string]int{}, strikes: map[string]loginStrike{}, known: map[string]time.Time{}}
}

// loginKey groups IPv6 clients by /64, the block one host usually controls.
func loginKey(client string) string {
	ip, err := netip.ParseAddr(client)
	if err != nil {
		return client
	}
	ip = ip.Unmap()
	if ip.Is6() {
		return netip.PrefixFrom(ip, 64).Masked().String()
	}
	return ip.String()
}

// allow records an attempt and reports whether it may proceed, or how long
// the client must wait.
func (l *loginLimiter) allow(key string, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.window) >= time.Minute {
		l.window, l.failures = now, 0
		clear(l.attempts)
	}
	if s, ok := l.strikes[key]; ok && now.Before(s.until) {
		return false, s.until.Sub(now)
	}
	known := now.Sub(l.known[key]) < loginKnownClientFor
	if l.attempts[key] >= loginAttemptsPerMinute || (!known && l.failures >= loginFailuresPerMinute) {
		return false, l.window.Add(time.Minute).Sub(now)
	}
	l.attempts[key]++
	return true, 0
}

// fail counts a rejected password. From the fifth failure in a day on, each
// failure locks the client out, for one minute and then twice as long each
// time, up to an hour.
func (l *loginLimiter) fail(key string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failures++
	s := l.strikes[key]
	if now.Sub(s.last) > loginStrikeMemory {
		s = loginStrike{}
	}
	s.count++
	s.last = now
	if s.count >= loginAttemptsPerMinute {
		lockout := loginMaxLockout
		if shift := s.count - loginAttemptsPerMinute; shift < 6 {
			lockout = min(time.Minute<<shift, loginMaxLockout)
		}
		s.until = now.Add(lockout)
	}
	if len(l.strikes) >= loginMaxClients {
		for k, v := range l.strikes {
			if now.After(v.until) && now.Sub(v.last) > time.Minute {
				delete(l.strikes, k)
			}
		}
	}
	if len(l.strikes) < loginMaxClients || l.strikes[key].count > 0 {
		l.strikes[key] = s
	}
}

// succeed clears the client's failures and exempts it from the global cap.
func (l *loginLimiter) succeed(key string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.strikes, key)
	if _, ok := l.known[key]; !ok && len(l.known) >= loginMaxKnownClients {
		oldest, first := "", true
		for k, at := range l.known {
			if first || at.Before(l.known[oldest]) {
				oldest, first = k, false
			}
		}
		delete(l.known, oldest)
	}
	l.known[key] = now
}
