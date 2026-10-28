package models

import (
	"bytes"
	"encoding/json"
	"html"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// MarshalJSON keeps tracker credentials out of API responses and events: the
// download link is replaced by HasDownloadURL and link fields leave Raw. The
// database stores search results with the link (see database.storedResult);
// adding a torrent looks it up there by result ID.
func (r Result) MarshalJSON() ([]byte, error) {
	type plain Result
	p := plain(r)
	p.HasDownloadURL = r.DownloadURL != "" || r.HasDownloadURL
	p.DownloadURL = ""
	p.InfoURL = PublicInfoURL(r.InfoURL, r.DownloadURL)
	p.Raw = PublicRaw(r.Raw, r.DownloadURL)
	return json.Marshal(p)
}

// rawLinkKeys name upstream fields that hold a download or magnet link.
var rawLinkKeys = map[string]bool{
	"download_url": true, "downloadurl": true, "link": true, "magnet": true,
	"magnet_url": true, "magneturl": true, "magnet_uri": true, "magneturi": true,
	"torrent_url": true, "torrenturl": true, "enclosure": true,
}

// rawSecretMarkers appear in tracker links that authenticate the user.
var rawSecretMarkers = []string{"apikey", "api_key", "passkey", "torrent_pass", "authkey", "rsskey", "token="}

// credentialWords are the words of query keys that carry a tracker credential.
var credentialWords = map[string]bool{
	"pass": true, "passkey": true, "password": true, "passwd": true, "torrentpass": true,
	"key": true, "apikey": true, "authkey": true, "rsskey": true, "auth": true,
	"token": true, "secret": true, "sig": true, "signature": true, "pk": true,
}

// CredentialKey reports whether a query key names a tracker credential. Keys
// match by whole words, split at _, -, . and case changes, so torrent_pass
// and apiKey match but monkey and author do not.
func CredentialKey(key string) bool {
	var words []string
	var word []rune
	flush := func() {
		if len(word) > 0 {
			words = append(words, string(word))
			word = word[:0]
		}
	}
	runes := []rune(key)
	for i, r := range runes {
		if r == '_' || r == '-' || r == '.' || unicode.IsSpace(r) {
			flush()
			continue
		}
		if unicode.IsUpper(r) && i > 0 && (unicode.IsLower(runes[i-1]) || unicode.IsDigit(runes[i-1]) ||
			(unicode.IsUpper(runes[i-1]) && i+1 < len(runes) && unicode.IsLower(runes[i+1]))) {
			flush()
		}
		word = append(word, unicode.ToLower(r))
	}
	flush()
	return credentialWords[strings.Join(words, "")] || slices.ContainsFunc(words, func(w string) bool { return credentialWords[w] })
}

// queryKey finds the keys of key=value pairs in a link or query string.
var queryKey = regexp.MustCompile(`(?:^|[?&;#])([^=?&;#\s"'<>]+)=`)

// passkeyHex matches a 32-character hex run, the usual passkey length; a
// 40-character infohash does not match.
var passkeyHex = regexp.MustCompile(`(?i)(?:^|[^0-9a-f])[0-9a-f]{32}(?:$|[^0-9a-f])`)

// linkSegments are path segments, or file names without their extension, of
// download and feed links, which may carry a passkey in the path.
var linkSegments = map[string]bool{"download": true, "downloads": true, "dl": true, "rss": true}

// HasCredential reports whether text holds a credential query key or a
// passkey-length hex run. It reads the raw text after HTML unescaping, so
// pairs joined by &amp; or ; count too, which a URL query parser skips, and
// decodes percent-encoded keys such as pass%6Bey.
func HasCredential(text string) bool {
	text = html.UnescapeString(text)
	decoded, err := url.QueryUnescape(text)
	if err != nil {
		decoded = text
	}
	if passkeyHex.MatchString(text) || passkeyHex.MatchString(decoded) {
		return true
	}
	for _, match := range queryKey.FindAllStringSubmatch(text, -1) {
		key := match[1]
		if k, err := url.QueryUnescape(key); err == nil {
			key = k
		}
		if CredentialKey(key) {
			return true
		}
	}
	return false
}

// DownloadPath reports whether a URL path is a download or feed link.
func DownloadPath(path string) bool {
	for _, segment := range strings.Split(strings.ToLower(path), "/") {
		name, _, _ := strings.Cut(segment, ".")
		if linkSegments[segment] || linkSegments[name] {
			return true
		}
	}
	return false
}

// PublicInfoURL drops an info link that holds a credential, is a download or
// feed link, or is the download link itself.
func PublicInfoURL(info, link string) string {
	if info != "" && rawSecret(info, link) {
		return ""
	}
	return info
}

func rawSecret(value, link string) bool {
	// Feeds may HTML-escape links, such as &amp; between parameters.
	value, link = html.UnescapeString(value), html.UnescapeString(link)
	// A guid can be the link plus a fragment, such as link#x.
	if link != "" && (value == link || strings.HasPrefix(value, link+"#")) {
		return true
	}
	lower := strings.ToLower(value)
	for _, marker := range rawSecretMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	if u, err := url.Parse(strings.TrimSpace(value)); err == nil && (u.Scheme == "http" || u.Scheme == "https") && DownloadPath(u.EscapedPath()) {
		return true
	}
	return HasCredential(value)
}

// PublicRaw removes link fields, and any string holding a credential or the
// download link itself, from an upstream result. Invalid JSON is dropped.
func PublicRaw(raw json.RawMessage, link string) json.RawMessage {
	if len(raw) == 0 || string(raw) == "null" {
		return raw
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil
	}
	value, _ = publicValue(value, link)
	b, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return b
}

// publicValue reports false when the whole value must be dropped.
func publicValue(value any, link string) (any, bool) {
	switch v := value.(type) {
	case string:
		return v, !rawSecret(v, link)
	case map[string]any:
		for key, inner := range v {
			if rawLinkKeys[strings.ToLower(key)] {
				delete(v, key)
				continue
			}
			if kept, ok := publicValue(inner, link); ok {
				v[key] = kept
			} else {
				delete(v, key)
			}
		}
		return v, true
	case []any:
		out := v[:0]
		for _, inner := range v {
			if kept, ok := publicValue(inner, link); ok {
				out = append(out, kept)
			}
		}
		return out, true
	}
	return value, true
}
