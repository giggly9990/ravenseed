package main

import (
	"net"
	"path/filepath"
	"strconv"
	"testing"
)

func TestListenFailureExitsWithError(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	t.Setenv("DATABASE_PATH", filepath.Join(t.TempDir(), "server.db"))
	t.Setenv("LISTEN_ADDR", "127.0.0.1")
	t.Setenv("LISTEN_PORT", strconv.Itoa(busy.Addr().(*net.TCPAddr).Port))
	t.Setenv("WEB_DIR", t.TempDir())
	for _, key := range []string{"AUTH_MODE", "API_TOKEN", "WEB_USERNAME", "WEB_PASSWORD", "WEB_PASSWORD_HASH", "QUI_URL", "QUI_API_KEY", "TMDB_TOKEN", "PUBLISHED_BIND_ADDRESS", "ALLOWED_HOSTS", "TRUSTED_PROXIES"} {
		t.Setenv(key, "")
	}
	if code := run(); code != 1 {
		t.Fatalf("exit code %d, want 1", code)
	}
}

func TestHealthcheckFailsWithoutServer(t *testing.T) {
	free, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := free.Addr().(*net.TCPAddr).Port
	free.Close()
	t.Setenv("LISTEN_ADDR", "0.0.0.0")
	t.Setenv("LISTEN_PORT", strconv.Itoa(port))
	if healthcheck() != 1 {
		t.Fatal("healthcheck passed without a server")
	}
}
