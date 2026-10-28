package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"ravenseed/internal/api"
	"ravenseed/internal/auth"
	"ravenseed/internal/config"
	"ravenseed/internal/database"
	"ravenseed/internal/search"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "version" || os.Args[1] == "--version") {
		fmt.Println(version)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "hash-password" {
		body, err := io.ReadAll(io.LimitReader(os.Stdin, 1026))
		if err != nil {
			slog.Error("could not read password from stdin")
			os.Exit(1)
		}
		hash, err := auth.HashPassword(strings.TrimSuffix(strings.TrimSuffix(string(body), "\n"), "\r"))
		if err != nil {
			slog.Error("could not hash password", "error", err)
			os.Exit(1)
		}
		fmt.Println(hash)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck())
	}
	os.Exit(run())
}

// healthcheck probes the listener the server binds: loopback for a wildcard
// or unset LISTEN_ADDR, else that address.
func healthcheck() int {
	port := os.Getenv("LISTEN_PORT")
	if port == "" {
		port = "8080"
	}
	host := os.Getenv("LISTEN_ADDR")
	if address, err := netip.ParseAddr(host); host == "" || (err == nil && address.IsUnspecified()) {
		host = "127.0.0.1"
	}
	c := http.Client{Timeout: 4 * time.Second}
	r, err := c.Get("http://" + net.JoinHostPort(host, port) + "/api/v1/health")
	if err != nil {
		return 1
	}
	r.Body.Close()
	if r.StatusCode != 200 {
		return 1
	}
	return 0
}

// webDir is WEB_DIR, else web/dist under the working directory (a source
// checkout), else web next to the executable (a release archive).
func webDir() string {
	if web := os.Getenv("WEB_DIR"); web != "" {
		return web
	}
	if _, err := os.Stat("web/dist"); err == nil {
		return "web/dist"
	}
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), "web")
	}
	return "web/dist"
}

func run() int {
	c, err := config.Load()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		return 1
	}
	var level slog.Level
	if err = level.UnmarshalText([]byte(c.LogLevel)); err != nil {
		slog.Error("invalid LOG_LEVEL")
		return 1
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})))
	if host, _, err := net.SplitHostPort(c.ListenAddr); err == nil && !c.AuthRequired() && !config.IsLoopbackHost(host) {
		slog.Warn("authentication is disabled on a non-loopback listener; other containers on shared Docker networks and hosts that reach this port can use the API", "address", c.ListenAddr)
	}
	db, err := database.Open(c.DatabasePath)
	if err != nil {
		slog.Error("database initialization failed", "error", err)
		return 1
	}
	defer db.DB.Close()
	s, err := search.New(db, c)
	if err != nil {
		slog.Error("service initialization failed", "error", err)
		return 1
	}
	web := webDir()
	s.StartUploads()
	s.StartMonitors()
	// Request contexts end shortly after shutdown starts, so event streams
	// close instead of holding the shutdown to its deadline.
	requests, endRequests := context.WithCancel(context.Background())
	defer endRequests()
	// Responses get a write deadline past the longest upstream wait a handler
	// allows; event streams extend their own deadline before every write.
	writeTimeout := max(c.SearchTimeout, 2*time.Minute) + time.Minute
	server := &http.Server{Addr: c.ListenAddr, Handler: api.New(s, web), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: writeTimeout, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10, BaseContext: func(net.Listener) context.Context { return requests }}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	failed := make(chan error, 1)
	go func() {
		slog.Info("server listening", "address", c.ListenAddr, "version", version)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			failed <- err
			stop()
		}
	}()
	<-ctx.Done()
	// Stop accepting requests and let handlers finish before the service and
	// database they use close.
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	grace := time.AfterFunc(5*time.Second, endRequests)
	defer grace.Stop()
	if err := server.Shutdown(shutdown); err != nil {
		slog.Warn("HTTP shutdown incomplete", "error", err)
		_ = server.Close()
	}
	s.Close()
	select {
	case err := <-failed:
		slog.Error("HTTP server failed", "error", err)
		return 1
	default:
		return 0
	}
}
