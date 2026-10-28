package config

import (
	"fmt"
	"net"
	"net/netip"
	"os"
	"ravenseed/internal/auth"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type Config struct {
	TMDbToken, TMDbBaseURL                                                 string
	QuiURL, APIKey, DatabasePath, ListenAddr, LogLevel, AuthMode, APIToken string
	DefaultTarget, DefaultCoverage, Concurrency, MaxJobs                   int
	SearchTimeout, CacheTTL                                                time.Duration
	UploadConcurrency                                                      int
	UploadSubmissionInterval                                               time.Duration
	HistoryRetention, RawRetention                                         time.Duration
	TrustedProxies                                                         []netip.Prefix
	// AllowedHosts are extra Host header names accepted while authentication
	// is disabled; loopback names are always accepted.
	AllowedHosts                 []string
	WebUsername, WebPasswordHash string
	WebCookieSecure              bool
}

func (c Config) AuthRequired() bool {
	return c.AuthMode == "token" || c.AuthMode == "session" || c.WebPasswordHash != ""
}

func Load() (Config, error) {
	c := Config{QuiURL: os.Getenv("QUI_URL"), DatabasePath: value("DATABASE_PATH", "data/discovery.db"), ListenAddr: net.JoinHostPort(value("LISTEN_ADDR", "127.0.0.1"), value("LISTEN_PORT", "8080")), LogLevel: value("LOG_LEVEL", "info"), AuthMode: value("AUTH_MODE", "none")}
	publishedBindAddress := os.Getenv("PUBLISHED_BIND_ADDRESS")
	var err error
	c.TMDbBaseURL = value("TMDB_BASE_URL", "https://api.themoviedb.org/3")
	if c.TMDbToken, err = secret("TMDB_TOKEN"); err != nil {
		return c, err
	}
	if c.APIKey, err = secret("QUI_API_KEY"); err != nil {
		return c, err
	}
	if c.APIToken, err = secret("API_TOKEN"); err != nil {
		return c, err
	}
	c.WebUsername = os.Getenv("WEB_USERNAME")
	password, err := secret("WEB_PASSWORD")
	if err != nil {
		return c, err
	}
	if c.WebPasswordHash, err = secret("WEB_PASSWORD_HASH"); err != nil {
		return c, err
	}
	if password != "" && c.WebPasswordHash != "" {
		return c, fmt.Errorf("set WEB_PASSWORD or WEB_PASSWORD_HASH, not both")
	}
	if password != "" {
		if c.WebPasswordHash, err = auth.HashPassword(password); err != nil {
			return c, err
		}
	}
	if (c.WebUsername == "") != (c.WebPasswordHash == "") || len(c.WebUsername) > 128 || strings.ContainsAny(c.WebUsername, "\r\n\x00") {
		return c, fmt.Errorf("configure WEB_USERNAME together with a web password or password hash")
	}
	if c.WebPasswordHash != "" {
		if err := auth.ValidateHash(c.WebPasswordHash); err != nil {
			return c, err
		}
	}
	if c.WebCookieSecure, err = strconv.ParseBool(value("WEB_COOKIE_SECURE", "true")); err != nil {
		return c, fmt.Errorf("invalid WEB_COOKIE_SECURE")
	}
	for _, item := range []struct {
		key, fallback string
		dest          *int
	}{
		{"DEFAULT_TARGET_INDEXER", "0", &c.DefaultTarget}, {"DEFAULT_MIN_COVERAGE", "2", &c.DefaultCoverage}, {"MAX_CONCURRENT_SEARCHES", "4", &c.Concurrency}, {"MAX_ACTIVE_JOBS", "8", &c.MaxJobs},
		{"MAX_CONCURRENT_UPLOADS", "1", &c.UploadConcurrency},
	} {
		n, e := strconv.Atoi(value(item.key, item.fallback))
		if e != nil || n < 0 {
			return c, fmt.Errorf("invalid %s", item.key)
		}
		*item.dest = n
	}
	if c.Concurrency < 1 || c.Concurrency > 32 || c.MaxJobs < 1 || c.MaxJobs > 100 || c.DefaultCoverage < 1 || c.DefaultCoverage > 1000 {
		return c, fmt.Errorf("invalid concurrency, job limit or coverage")
	}
	if c.UploadConcurrency < 1 || c.UploadConcurrency > 4 {
		return c, fmt.Errorf("MAX_CONCURRENT_UPLOADS must be from 1 to 4")
	}
	if c.UploadSubmissionInterval, err = time.ParseDuration(value("UA_SUBMISSION_INTERVAL", "1m")); err != nil || c.UploadSubmissionInterval < time.Second {
		return c, fmt.Errorf("UA_SUBMISSION_INTERVAL must be at least 1s")
	}
	for _, item := range []struct {
		key  string
		dest *time.Duration
	}{{"HISTORY_RETENTION", &c.HistoryRetention}, {"RAW_RESULT_RETENTION", &c.RawRetention}} {
		if *item.dest, err = time.ParseDuration(value(item.key, "0")); err != nil || *item.dest < 0 {
			return c, fmt.Errorf("invalid %s", item.key)
		}
	}
	allowUnauthenticatedRemote, err := strconv.ParseBool(value("ALLOW_UNAUTHENTICATED_REMOTE", "false"))
	if err != nil {
		return c, fmt.Errorf("invalid ALLOW_UNAUTHENTICATED_REMOTE")
	}
	for _, text := range strings.Split(os.Getenv("ALLOWED_HOSTS"), ",") {
		if text = strings.TrimSpace(text); text == "" {
			continue
		}
		host := NormalizeHost(text)
		if host == "" || len(host) > 253 || strings.ContainsFunc(text, func(r rune) bool {
			return strings.ContainsRune("/\\@*?#", r) || unicode.IsSpace(r) || unicode.IsControl(r)
		}) || strings.ContainsAny(host, "[]") || (strings.Contains(host, ":") && !validIPv6(host)) {
			return c, fmt.Errorf("ALLOWED_HOSTS must contain host names or IP addresses, separated by commas")
		}
		c.AllowedHosts = append(c.AllowedHosts, host)
	}
	for _, text := range strings.Split(os.Getenv("TRUSTED_PROXIES"), ",") {
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		prefix, e := netip.ParsePrefix(text)
		if e != nil {
			address, e := netip.ParseAddr(text)
			if e != nil {
				return c, fmt.Errorf("TRUSTED_PROXIES must contain IP addresses or CIDRs")
			}
			prefix = netip.PrefixFrom(address, address.BitLen())
		}
		c.TrustedProxies = append(c.TrustedProxies, prefix.Masked())
	}
	if c.SearchTimeout, err = time.ParseDuration(value("SEARCH_TIMEOUT", "3m")); err != nil || c.SearchTimeout <= 0 {
		return c, fmt.Errorf("invalid SEARCH_TIMEOUT")
	}
	if c.CacheTTL, err = time.ParseDuration(value("RESULT_CACHE_TTL", "24h")); err != nil || c.CacheTTL < 0 {
		return c, fmt.Errorf("invalid RESULT_CACHE_TTL")
	}
	if c.AuthMode != "none" && c.AuthMode != "token" && c.AuthMode != "session" {
		return c, fmt.Errorf("AUTH_MODE must be none, session or token")
	}
	if c.APIToken != "" && c.AuthMode == "none" {
		// A token alone enables token mode; an explicit AUTH_MODE=none would
		// silently ignore it.
		if os.Getenv("AUTH_MODE") != "" {
			return c, fmt.Errorf("API_TOKEN is set but AUTH_MODE=none disables it; use AUTH_MODE=token or session, or remove API_TOKEN")
		}
		c.AuthMode = "token"
	}
	if (c.AuthMode == "token" || c.APIToken != "") && len(c.APIToken) < 24 {
		return c, fmt.Errorf("API_TOKEN must contain at least 24 characters")
	}
	if c.AuthMode == "session" && c.WebPasswordHash == "" {
		return c, fmt.Errorf("AUTH_MODE=session requires WEB_USERNAME and a web password")
	}
	if publishedBindAddress != "" {
		if _, err := netip.ParseAddr(publishedBindAddress); err != nil {
			return c, fmt.Errorf("PUBLISHED_BIND_ADDRESS must be an IP address matching the host port binding")
		}
	}
	host, port, err := net.SplitHostPort(c.ListenAddr)
	if err != nil {
		return c, fmt.Errorf("invalid listen address")
	}
	p, err := strconv.Atoi(port)
	if err != nil || p < 1 || p > 65535 {
		return c, fmt.Errorf("invalid LISTEN_PORT")
	}
	if publishedBindAddress != "" {
		host = publishedBindAddress
	}
	loopback := IsLoopbackHost(host)
	if !c.WebCookieSecure && !loopback {
		return c, fmt.Errorf("WEB_COOKIE_SECURE=false is allowed only with a loopback listener or host publication")
	}
	if !c.AuthRequired() && !loopback && !allowUnauthenticatedRemote {
		return c, fmt.Errorf("remote binding requires AUTH_MODE=token or explicit ALLOW_UNAUTHENTICATED_REMOTE=true; Docker users can declare a loopback PUBLISHED_BIND_ADDRESS matching their host port binding")
	}
	return c, nil
}

// NormalizeHost reduces a Host header or host name to a lowercase name
// without port, IPv6 brackets or trailing dot.
func NormalizeHost(host string) string {
	host = strings.TrimSpace(host)
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	return strings.ToLower(strings.TrimSuffix(host, "."))
}

func validIPv6(host string) bool {
	address, err := netip.ParseAddr(host)
	return err == nil && address.Is6()
}

// IsLoopbackHost reports localhost or a loopback IP address.
func IsLoopbackHost(host string) bool {
	host = NormalizeHost(host)
	if host == "localhost" {
		return true
	}
	address, err := netip.ParseAddr(host)
	return err == nil && address.IsLoopback()
}

func value(key, fallback string) string {
	if s := os.Getenv(key); s != "" {
		return s
	}
	return fallback
}
func secret(key string) (string, error) {
	if path := os.Getenv(key + "_FILE"); path != "" {
		b, err := os.ReadFile(path)
		return strings.TrimSpace(string(b)), err
	}
	return os.Getenv(key), nil
}
