package search

import (
	"context"
	"database/sql"
	"errors"
)

// TimeoutHint is a timeout whose hint tells the user what to check, such as
// an add that qui may have done anyway.
type TimeoutHint struct {
	Hint string
	Err  error
}

func (e TimeoutHint) Error() string { return e.Hint }
func (e TimeoutHint) Unwrap() error { return e.Err }

// UpstreamError marks a failure of qui, Sonarr, Radarr, TMDb or
// Upload-Assistant. Its message is meant for the user.
type UpstreamError struct{ Err error }

func (e UpstreamError) Error() string { return e.Err.Error() }
func (e UpstreamError) Unwrap() error { return e.Err }

// InternalError marks a store or other server-side failure. Its message is
// logged, not shown to clients.
type InternalError struct{ Err error }

func (e InternalError) Error() string { return e.Err.Error() }
func (e InternalError) Unwrap() error { return e.Err }

func upstream(err error) error {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var u UpstreamError
	if errors.As(err, &u) {
		return err
	}
	return UpstreamError{err}
}

// internal marks a store error; a missing row stays recognizable.
func internal(err error) error {
	if err == nil || errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var i InternalError
	if errors.As(err, &i) {
		return err
	}
	return InternalError{err}
}
