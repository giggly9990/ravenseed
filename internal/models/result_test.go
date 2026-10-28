package models

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestResultJSONLeavesOutTrackerCredentials(t *testing.T) {
	link := "https://tracker.example/dl/9?id=1&sig=abc"
	raw := `{"title":"Movie","size":9007199254740993,"download_url":"` + link + `","Link":"x","magnet_url":"magnet:?xt=1","guid":"` + link + `#x",` +
		`"rss":"https://t.example/rss?id=1&token=other",` +
		`"escaped":"https://tracker.example/dl/9?id=1&amp;sig=abc","signed":"https://t.example/f?id=2&sig=s2","keyed":"https://t.example/f?id=3&key=k3",` +
		`"pk":"https://t.example/f?id=4&pk=p4","monkey":"https://t.example/m?monkey=kept","author":"https://t.example/a?author=kept",` +
		`"info_url":"https://tracker.example/details/9","comments":"https://t.example/c?passkey=secret",` +
		`"nested":{"enclosure":{"url":"y"},"keep":"ok","list":["fine","https://t.example/rss?apikey=secret"]}}`
	b, err := json.Marshal(Result{ID: "r", DownloadURL: link, Raw: json.RawMessage(raw)})
	if err != nil {
		t.Fatal(err)
	}
	body := string(b)
	for _, leak := range []string{"downloadUrl", "download_url", "sig=abc", "token=other", "s2", "k3", "p4", `"guid"`, "magnet", `"Link"`, "passkey", "apikey", "enclosure"} {
		if strings.Contains(body, leak) {
			t.Fatalf("%q leaked: %s", leak, body)
		}
	}
	for _, kept := range []string{`"hasDownloadUrl":true`, `"info_url":"https://tracker.example/details/9"`, `"keep":"ok"`, `"list":["fine"]`, `9007199254740993`, `"title":"Movie"`, "monkey=kept", "author=kept"} {
		if !strings.Contains(body, kept) {
			t.Fatalf("%q missing: %s", kept, body)
		}
	}
	// Passkeys in a download path, a bare passkey, and pairs a URL parser skips.
	for _, secret := range []string{"https://p.example/torrents/download/123.0123456789abcdef0123456789abcdef", "0123456789abcdef0123456789abcdef",
		"https://p.example/download.php?id=1", "https://p.example/t?id=1;passkey=abc", "https://p.example/t?id=1&amp;passkey=abc",
		"https://p.example/t?pass%6Bey=abc", "https://p.example/t?%70asskey=abc"} {
		if got := string(PublicRaw(json.RawMessage(`{"comments":"`+secret+`","keep":"ok"}`), link)); got != `{"keep":"ok"}` {
			t.Fatalf("%q kept: %s", secret, got)
		}
	}
	// The top-level info link follows the same rules as raw.
	for info, want := range map[string]string{
		"https://p.example/details/9":     "https://p.example/details/9",
		"https://p.example/t?pass%6Bey=x": "",
		link:                              "",
	} {
		b, _ := json.Marshal(Result{ID: "i", DownloadURL: link, InfoURL: info})
		var back Result
		if err := json.Unmarshal(b, &back); err != nil || back.InfoURL != want {
			t.Fatalf("info link %q: %q %v", info, back.InfoURL, err)
		}
	}
	if b, _ := json.Marshal(PolicyTargetResult{ID: "t", InfoURL: "https://p.example/t?passkey=x"}); strings.Contains(string(b), "passkey") {
		t.Fatalf("target info link kept: %s", b)
	}
	// An HTML-escaped copy of the link is the link.
	if got := string(PublicRaw(json.RawMessage(`{"guid":"https://t.example/f/9?id=1&amp;x=y#1","keep":"ok"}`), "https://t.example/f/9?id=1&x=y")); got != `{"keep":"ok"}` {
		t.Fatalf("escaped link kept: %s", got)
	}
	var back Result
	if err := json.Unmarshal(b, &back); err != nil || !back.HasDownloadURL || back.DownloadURL != "" {
		t.Fatalf("round trip: %+v %v", back, err)
	}
	// A result restored without its link (a stored family) still reports one.
	if b, _ = json.Marshal(back); !strings.Contains(string(b), `"hasDownloadUrl":true`) {
		t.Fatalf("link flag lost: %s", b)
	}
	if b, _ = json.Marshal(Result{ID: "none"}); !strings.Contains(string(b), `"hasDownloadUrl":false`) || !strings.Contains(string(b), `"raw":null`) {
		t.Fatalf("empty result: %s", b)
	}
}

func TestCredentialKey(t *testing.T) {
	for key, want := range map[string]bool{
		"passkey": true, "torrent_pass": true, "torrentPass": true, "apiKey": true, "API_KEY": true, "authkey": true,
		"auth": true, "access_token": true, "secret": true, "sig": true, "signature": true, "pk": true, "rsskey": true, "key": true,
		"monkey": false, "author": false, "designation": false, "keyword": false, "id": false, "uid": false, "hash": false,
	} {
		if CredentialKey(key) != want {
			t.Errorf("CredentialKey(%q) = %v", key, !want)
		}
	}
}
