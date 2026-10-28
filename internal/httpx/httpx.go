// Package httpx holds the HTTP plumbing shared by the qui, Sonarr/Radarr,
// TMDb and Upload-Assistant clients.
package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"
)

// ValidBaseURL reports whether raw is an HTTP or HTTPS base URL with a host
// and without credentials, query parameters or a fragment.
func ValidBaseURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == ""
}

// NoRedirect refuses redirects, so credentials never follow a Location header
// to another host.
func NoRedirect(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }

// NewClient returns a client with an overall timeout that does not follow
// redirects.
func NewClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, CheckRedirect: NoRedirect}
}

// drainLimit bounds how much of an unread body is discarded to keep the
// connection reusable.
const drainLimit = 64 << 10

// Close discards what remains of a small response body and closes it, so the
// connection returns to the pool.
func Close(body io.ReadCloser) {
	_, _ = io.Copy(io.Discard, io.LimitReader(body, drainLimit))
	_ = body.Close()
}

// ErrInvalidJSON reports a body that is not JSON or exceeds the size limit.
var ErrInvalidJSON = errors.New("invalid or oversized JSON response")

// DecodeJSON decodes at most limit bytes of r into out.
func DecodeJSON(r io.Reader, limit int64, out any) error {
	if err := json.NewDecoder(io.LimitReader(r, limit)).Decode(out); err != nil {
		return ErrInvalidJSON
	}
	return nil
}

// RequestError returns the context's error when the request ended because
// ctx did, and fallback otherwise, so callers can tell cancellation from an
// unreachable service.
func RequestError(ctx context.Context, fallback error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return fallback
}

// AuthRejected reports a status meaning the service refused the credential.
func AuthRejected(status int) bool {
	return status == http.StatusUnauthorized || status == http.StatusForbidden
}
