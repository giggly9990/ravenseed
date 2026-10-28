package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func cleanEnvironment(t *testing.T) {
	t.Helper()
	for _, key := range []string{"WEB_USERNAME", "WEB_PASSWORD", "WEB_PASSWORD_FILE", "WEB_PASSWORD_HASH", "WEB_PASSWORD_HASH_FILE", "WEB_COOKIE_SECURE"} {
		t.Setenv(key, "")
	}
	for _, key := range []string{"LISTEN_ADDR", "LISTEN_PORT", "AUTH_MODE", "API_TOKEN", "API_TOKEN_FILE", "QUI_URL", "QUI_API_KEY", "QUI_API_KEY_FILE", "TMDB_TOKEN", "TMDB_TOKEN_FILE", "DEFAULT_TARGET_INDEXER", "DEFAULT_MIN_COVERAGE", "MAX_CONCURRENT_SEARCHES", "MAX_ACTIVE_JOBS", "MAX_CONCURRENT_UPLOADS", "UA_SUBMISSION_INTERVAL", "SEARCH_TIMEOUT", "RESULT_CACHE_TTL", "PUBLISHED_BIND_ADDRESS", "ALLOW_UNAUTHENTICATED_REMOTE", "TRUSTED_PROXIES", "HISTORY_RETENTION", "RAW_RESULT_RETENTION", "ALLOWED_HOSTS"} {
		t.Setenv(key, "")
	}
}

func TestDeploymentBoundaries(t *testing.T) {
	for _, test := range []struct {
		name  string
		env   map[string]string
		valid bool
	}{
		{"local default", nil, true},
		{"IPv6 loopback", map[string]string{"LISTEN_ADDR": "::1"}, true},
		{"remote unauthenticated", map[string]string{"LISTEN_ADDR": "0.0.0.0"}, false},
		{"docker localhost publication", map[string]string{"LISTEN_ADDR": "0.0.0.0", "PUBLISHED_BIND_ADDRESS": "127.0.0.1"}, true},
		{"docker remote publication", map[string]string{"LISTEN_ADDR": "0.0.0.0", "PUBLISHED_BIND_ADDRESS": "0.0.0.0"}, false},
		{"remote token", map[string]string{"LISTEN_ADDR": "0.0.0.0", "AUTH_MODE": "token", "API_TOKEN": "012345678901234567890123"}, true},
		{"short token", map[string]string{"AUTH_MODE": "token", "API_TOKEN": "short"}, false},
		{"session needs credentials", map[string]string{"AUTH_MODE": "session"}, false},
		{"username without password", map[string]string{"WEB_USERNAME": "admin"}, false},
		{"password without username", map[string]string{"WEB_PASSWORD": "long-enough-password"}, false},
		{"session credentials", map[string]string{"AUTH_MODE": "session", "WEB_USERNAME": "admin", "WEB_PASSWORD": "long-enough-password"}, true},
		{"remote session", map[string]string{"LISTEN_ADDR": "0.0.0.0", "WEB_USERNAME": "admin", "WEB_PASSWORD": "long-enough-password"}, true},
		{"remote insecure cookies", map[string]string{"LISTEN_ADDR": "0.0.0.0", "WEB_COOKIE_SECURE": "false", "ALLOW_UNAUTHENTICATED_REMOTE": "true"}, false},
		{"local HTTP cookie", map[string]string{"WEB_COOKIE_SECURE": "false", "WEB_USERNAME": "admin", "WEB_PASSWORD": "long-enough-password"}, true},
		{"explicit remote opt in", map[string]string{"LISTEN_ADDR": "0.0.0.0", "ALLOW_UNAUTHENTICATED_REMOTE": "true"}, true},
		{"invalid opt in", map[string]string{"ALLOW_UNAUTHENTICATED_REMOTE": "maybe"}, false},
		{"invalid publication", map[string]string{"PUBLISHED_BIND_ADDRESS": "localhost:8088"}, false},
		{"invalid port", map[string]string{"LISTEN_PORT": "65536"}, false},
		{"invalid retention", map[string]string{"HISTORY_RETENTION": "-1h"}, false},
		{"invalid raw retention", map[string]string{"RAW_RESULT_RETENTION": "tomorrow"}, false},
		{"invalid upload workers", map[string]string{"MAX_CONCURRENT_UPLOADS": "0"}, false},
		{"excess upload workers", map[string]string{"MAX_CONCURRENT_UPLOADS": "5"}, false},
		{"short upload spacing", map[string]string{"UA_SUBMISSION_INTERVAL": "500ms"}, false},
		{"valid upload spacing", map[string]string{"UA_SUBMISSION_INTERVAL": "2m"}, true},
		{"trusted proxy CIDRs", map[string]string{"TRUSTED_PROXIES": "127.0.0.1,10.0.0.0/8,::1"}, true},
		{"untrusted proxy name", map[string]string{"TRUSTED_PROXIES": "proxy.local"}, false},
		{"allowed hosts", map[string]string{"ALLOWED_HOSTS": "nas.lan, ravenseed:8080,192.168.1.5,[fd00::5]"}, true},
		{"allowed host wildcard", map[string]string{"ALLOWED_HOSTS": "*"}, false},
		{"allowed host URL", map[string]string{"ALLOWED_HOSTS": "http://nas.lan/"}, false},
		{"token without mode", map[string]string{"API_TOKEN": "012345678901234567890123", "LISTEN_ADDR": "0.0.0.0"}, true},
		{"token with explicit none", map[string]string{"AUTH_MODE": "none", "API_TOKEN": "012345678901234567890123"}, false},
		{"coverage too large", map[string]string{"DEFAULT_MIN_COVERAGE": "1001"}, false},
		{"coverage zero", map[string]string{"DEFAULT_MIN_COVERAGE": "0"}, false},
		{"password and hash", map[string]string{"WEB_USERNAME": "admin", "WEB_PASSWORD": "long-enough-password", "WEB_PASSWORD_HASH": "pbkdf2-sha256$600000$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}, false},
		{"invalid hash", map[string]string{"WEB_USERNAME": "admin", "WEB_PASSWORD_HASH": "plaintext"}, false},
		{"valid hash", map[string]string{"WEB_USERNAME": "admin", "WEB_PASSWORD_HASH": "pbkdf2-sha256$600000$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}, true},
		{"long username", map[string]string{"WEB_USERNAME": strings.Repeat("a", 129), "WEB_PASSWORD": "long-enough-password"}, false},
		{"username with newline", map[string]string{"WEB_USERNAME": "ad\nmin", "WEB_PASSWORD": "long-enough-password"}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			cleanEnvironment(t)
			for key, value := range test.env {
				t.Setenv(key, value)
			}
			_, err := Load()
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v, err=%v", test.valid, err)
			}
		})
	}
}

func TestSecretFileAndDefaults(t *testing.T) {
	cleanEnvironment(t)
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte(" 012345678901234567890123\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("API_TOKEN_FILE", path)
	t.Setenv("API_TOKEN", "ignored")
	t.Setenv("AUTH_MODE", "token")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.APIToken != "012345678901234567890123" || c.ListenAddr != "127.0.0.1:8080" || c.UploadConcurrency != 1 || c.UploadSubmissionInterval != time.Minute || c.HistoryRetention != 0 {
		t.Fatal("unexpected defaults or secret precedence")
	}
	t.Setenv("API_TOKEN_FILE", path+"-missing")
	if _, err := Load(); err == nil {
		t.Fatal("missing secret file accepted")
	}
}

func TestAllowedHostsAndImpliedTokenMode(t *testing.T) {
	cleanEnvironment(t)
	t.Setenv("ALLOWED_HOSTS", "NAS.lan., ravenseed:8080, [fd00::5]:80")
	t.Setenv("API_TOKEN", "012345678901234567890123")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(c.AllowedHosts, ",") != "nas.lan,ravenseed,fd00::5" {
		t.Fatalf("hosts: %v", c.AllowedHosts)
	}
	if c.AuthMode != "token" || !c.AuthRequired() {
		t.Fatal("API_TOKEN alone did not enable token mode")
	}
	for host, loopback := range map[string]bool{"localhost:80": true, "[::1]:8080": true, "127.0.0.9": true, "::1": true, "nas.lan": false, "": false, "localhost.example": false} {
		if IsLoopbackHost(host) != loopback {
			t.Fatalf("IsLoopbackHost(%q) != %v", host, loopback)
		}
	}
}
