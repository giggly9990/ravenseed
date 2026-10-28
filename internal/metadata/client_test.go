package metadata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTokenNeverFollowsRedirect(t *testing.T) {
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("TMDb token followed a redirect") }))
	defer receiver.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, receiver.URL, http.StatusFound) }))
	defer server.Close()
	for _, credential := range []string{"private-token", "0123456789abcdef0123456789abcdef"} {
		c, _ := New(server.URL, credential, "en-US")
		if err := c.Test(context.Background()); err == nil || strings.Contains(err.Error(), credential) {
			t.Fatal("unsafe redirect handling")
		}
	}
}

func TestCredentialAuthentication(t *testing.T) {
	for _, tc := range []struct {
		name, input, key, bearer string
	}{
		{"API key", " 0123456789abcdef0123456789abcdef ", "0123456789abcdef0123456789abcdef", ""},
		{"read token", " private-token ", "", "Bearer private-token"},
		{"pasted bearer", "Bearer private-token", "", "Bearer private-token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("api_key") != tc.key || r.Header.Get("Authorization") != tc.bearer {
					t.Error("wrong credential authentication method")
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				if r.URL.Query().Get("language") != "fr" {
					t.Error("language was lost")
				}
				_, _ = w.Write([]byte(`{}`))
			}))
			defer server.Close()
			c, err := New(server.URL, tc.input, "fr")
			if err != nil {
				t.Fatal(err)
			}
			if err := c.Test(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestUnauthorizedMessageDoesNotExposeCredential(t *testing.T) {
	const key = "0123456789abcdef0123456789abcdef"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(r.URL.String()))
	}))
	defer server.Close()
	c, _ := New(server.URL, key, "en-US")
	err := c.Test(context.Background())
	if err == nil || !strings.Contains(err.Error(), "HTTP 401") || !strings.Contains(err.Error(), "API key or API Read Access Token") || strings.Contains(err.Error(), key) {
		t.Fatal("expected actionable, redacted authentication error")
	}
}

func TestImagePathsAndLanguage(t *testing.T) {
	for _, path := range []string{"https://other.example/poster.jpg", "//other.example/poster.jpg", "/../secret", "/poster.jpg?token=x"} {
		if imagePath(path) != "" {
			t.Fatal("untrusted image path accepted")
		}
	}
	if imagePath("/poster.jpg") == "" {
		t.Fatal("valid poster rejected")
	}
	if _, err := Language("en-US&token=secret"); err == nil {
		t.Fatal("invalid language accepted")
	}
}
