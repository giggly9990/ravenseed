package httpx

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestValidBaseURL(t *testing.T) {
	for raw, want := range map[string]bool{"http://qui:7476": true, "https://sonarr.lan/base": true, "ftp://host": false, "http://user:pass@host": false, "http://host/?q=1": false, "http://host/#x": false, "/relative": false} {
		if got := ValidBaseURL(raw); got != want {
			t.Errorf("%s: %v", raw, got)
		}
	}
}

func TestDecodeJSONAndRequestError(t *testing.T) {
	var out map[string]int
	if err := DecodeJSON(strings.NewReader(`{"a":1}`), 1<<10, &out); err != nil || out["a"] != 1 {
		t.Fatalf("decode: %v %v", out, err)
	}
	if err := DecodeJSON(strings.NewReader(`{"a":1}`), 3, &out); !errors.Is(err, ErrInvalidJSON) {
		t.Fatalf("oversized body: %v", err)
	}
	fallback := errors.New("unreachable")
	ctx, cancel := context.WithCancel(context.Background())
	if err := RequestError(ctx, fallback); err != fallback {
		t.Fatalf("live context: %v", err)
	}
	cancel()
	if err := RequestError(ctx, fallback); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled context: %v", err)
	}
}
