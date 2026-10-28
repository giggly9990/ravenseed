package uploads

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ravenseed/internal/models"
)

func uaServer(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	s := httptest.NewServer(handler)
	t.Cleanup(s.Close)
	c, err := New(s.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

var uploadJob = models.UploadJob{ID: "job", Tracker: "AITHER", UploadPath: "/data/movie"}

func TestExecutionStreamFailures(t *testing.T) {
	for _, tc := range []struct {
		name, contentType, body, want string
	}{
		{"not a stream", "application/json", `{"success":true}`, "did not return an execution stream"},
		{"missing exit code", "text/event-stream", "data: {\"type\":\"exit\"}\n\n", "missing Upload-Assistant exit code"},
		{"oversized line", "text/event-stream", "data: " + strings.Repeat("x", 2<<20) + "\n\n", "oversized Upload-Assistant event"},
		{"no completion", "text/event-stream", "data: {\"type\":\"progress\"}\n\n", "without a completion event"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := uaServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				_, _ = w.Write([]byte(tc.body))
			})
			_, err := c.Execute(context.Background(), uploadJob, func(string) error { return nil })
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
		})
	}
}

func TestStalledStreamIsAbandoned(t *testing.T) {
	previous := streamIdle
	streamIdle = 50 * time.Millisecond
	t.Cleanup(func() { streamIdle = previous })
	release := make(chan struct{})
	c := uaServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"progress\"}\n\n"))
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
	// Registered after the server, so it runs before the server closes.
	t.Cleanup(func() { close(release) })
	start := time.Now()
	_, err := c.Execute(context.Background(), uploadJob, func(string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "sent nothing") || time.Since(start) > 5*time.Second {
		t.Fatalf("stalled stream: %v after %s", err, time.Since(start))
	}
}

func TestRateLimitRetryAfterIsCapped(t *testing.T) {
	c := uaServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "999999999999")
		w.WriteHeader(http.StatusTooManyRequests)
	})
	_, err := c.Execute(context.Background(), uploadJob, func(string) error { return nil })
	var limited *RateLimitError
	if !errors.As(err, &limited) || limited.RetryAfter != maxRetryAfter {
		t.Fatalf("rate limit: %v", err)
	}
	for value, want := range map[string]time.Duration{"30": 30 * time.Second, "0": 0, "-5": 0, "soon": 0} {
		if got := retryAfter(value); got != want {
			t.Errorf("Retry-After %q: %s, want %s", value, got, want)
		}
	}
}

func TestErrorResponsesStayGeneric(t *testing.T) {
	for status, want := range map[int]string{http.StatusUnauthorized: "rejected the API token", http.StatusTeapot: "returned an error response"} {
		c := uaServer(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) })
		if err := c.Test(context.Background()); err == nil || !strings.Contains(err.Error(), want) || strings.Contains(err.Error(), "418") {
			t.Errorf("HTTP %d: %v", status, err)
		}
	}
}

func TestSingleDashAbbreviationsAreReserved(t *testing.T) {
	for _, args := range []string{"-tmd movie/1", "-imd tt0133093", "-tm=movie/1", "-uac", "--tmd movie/1"} {
		if _, err := NormalizeArgs(args); err == nil {
			t.Errorf("%s accepted", args)
		}
	}
	for _, args := range []string{"-d", "--debug", "-sk"} {
		if _, err := NormalizeArgs(args); err != nil {
			t.Errorf("%s rejected: %v", args, err)
		}
	}
}

func TestMapPathDriveRoot(t *testing.T) {
	v, err := Validate(models.UploadSettings{Paths: []models.UploadPath{{InstanceID: 1, From: `D:\`, To: "/d"}, {InstanceID: 1, From: "/", To: "/root"}}})
	if err != nil {
		t.Fatal(err)
	}
	for from, want := range map[string]string{`d:\Movies\a.mkv`: "/d/Movies/a.mkv", "/srv/a.mkv": "/root/srv/a.mkv"} {
		if got, err := MapPath(from, 1, v.Paths); err != nil || got != want {
			t.Errorf("%s: %s %v, want %s", from, got, err, want)
		}
	}
}
