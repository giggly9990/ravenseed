package api

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"

	"ravenseed/internal/search"
)

// serviceError answers a failed service call: a missing record is 404, a
// store or other server failure 500 without its details, a timeout 504, an
// upstream failure 502, and anything else, a rejected input, 400 with its
// message.
func serviceError(w http.ResponseWriter, err error, code, notFound string) {
	var internal search.InternalError
	var upstream search.UpstreamError
	switch {
	case errors.Is(err, sql.ErrNoRows):
		fail(w, 404, "not_found", notFound)
	case errors.As(err, &internal):
		slog.Error("request failed", "code", code, "error", err)
		fail(w, 500, "internal_error", "The server could not complete the request; see its log")
	case errors.Is(err, context.DeadlineExceeded):
		message := "The request timed out"
		var hint search.TimeoutHint
		if errors.As(err, &hint) {
			message += ". " + hint.Hint
		}
		fail(w, 504, "timeout", message)
	case errors.As(err, &upstream):
		fail(w, 502, code, err.Error())
	default:
		fail(w, 400, code, err.Error())
	}
}

// storeError answers a failed store read: 404 for a missing record, else 500.
func storeError(w http.ResponseWriter, err error, notFound, failed string) {
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, 404, "not_found", notFound)
		return
	}
	slog.Error("database request failed", "error", err)
	fail(w, 500, "database_error", failed)
}
