package uploads

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"path"
	"ravenseed/internal/httpx"
	"ravenseed/internal/models"
	"ravenseed/internal/releases"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode"
)

var trackerPattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_-]{0,39}$`)
var drivePattern = regexp.MustCompile(`^[A-Za-z]:/`)

func Validate(v models.UploadSettings) (models.UploadSettings, error) {
	if err := validatePresets(&v); err != nil {
		return v, err
	}
	v.URL = strings.TrimRight(strings.TrimSpace(v.URL), "/")
	v.Token = strings.TrimSpace(v.Token)
	if v.URL != "" {
		if !httpx.ValidBaseURL(v.URL) {
			return v, errors.New("use an HTTP or HTTPS Upload-Assistant URL without credentials or query parameters")
		}
	}
	if len(v.Token) > 4096 || strings.ContainsAny(v.Token, "\r\n") {
		return v, errors.New("invalid Upload-Assistant token")
	}
	if v.Enabled && (v.URL == "" || v.Token == "") {
		return v, errors.New("enter the Upload-Assistant URL and API token before enabling uploads")
	}
	if len(v.Trackers) > 1000 || len(v.Paths) > 100 {
		return v, errors.New("too many upload mappings")
	}
	seen := map[int]bool{}
	for i := range v.Trackers {
		t := &v.Trackers[i]
		t.Code = strings.ToUpper(strings.TrimSpace(t.Code))
		if t.IndexerID <= 0 || seen[t.IndexerID] || !trackerPattern.MatchString(t.Code) {
			return v, errors.New("map each target indexer once to a valid Upload-Assistant tracker code")
		}
		seen[t.IndexerID] = true
	}
	paths := map[string]bool{}
	for i := range v.Paths {
		p := &v.Paths[i]
		var err error
		p.From, err = cleanPath(p.From)
		if err != nil {
			return v, err
		}
		p.To, err = cleanPath(p.To)
		if err != nil {
			return v, err
		}
		key := fmt.Sprintf("%d:%s", p.InstanceID, p.From)
		if p.InstanceID <= 0 || paths[key] {
			return v, errors.New("use one path mapping per instance and source root")
		}
		paths[key] = true
	}
	v.HasToken, v.ClearToken = false, false
	return v, nil
}

func cleanPath(value string) (string, error) {
	v := strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	if len(v) > 4096 || strings.ContainsAny(v, "\x00\r\n") || (!strings.HasPrefix(v, "/") && !drivePattern.MatchString(v)) {
		return "", errors.New("path mappings must use absolute paths")
	}
	for _, part := range strings.Split(v, "/") {
		if part == ".." {
			return "", errors.New("parent traversal is not allowed in upload paths")
		}
	}
	if strings.HasPrefix(v, "//") {
		return "", errors.New("mount network shares locally and use their absolute mount paths")
	}
	if len(v) == 3 && drivePattern.MatchString(v) {
		return v, nil
	}
	return path.Clean(v), nil
}

func MapPath(value string, instance int, mappings []models.UploadPath) (string, error) {
	v, err := cleanPath(value)
	if err != nil {
		return "", err
	}
	best, mapped := -1, ""
	for _, p := range mappings {
		if p.InstanceID != instance {
			continue
		}
		from := strings.TrimRight(p.From, "/")
		compareV, compareFrom := v, from
		if drivePattern.MatchString(from + "/") {
			compareV, compareFrom = strings.ToLower(v), strings.ToLower(from)
		}
		if (compareV == compareFrom || strings.HasPrefix(compareV, compareFrom+"/")) && len(from) > best {
			best = len(from)
			mapped = strings.TrimRight(p.To, "/") + "/" + strings.TrimLeft(v[len(from):], "/")
		}
	}
	if best < 0 {
		return "", errors.New("completed download is outside the saved path mappings")
	}
	return cleanPath(mapped)
}

type Client struct {
	base, token string
	http        *http.Client
}

// RateLimitError means UA rejected the request before opening an execution stream.
// Other execution errors may have happened after UA started work and must not be retried.
type RateLimitError struct {
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string { return "Upload-Assistant returned HTTP 429" }

// maxRetryAfter caps the wait a Retry-After header can ask for.
const maxRetryAfter = 24 * time.Hour

func retryAfter(value string) time.Duration {
	if seconds, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil && seconds > 0 {
		return time.Duration(min(seconds, int64(maxRetryAfter/time.Second))) * time.Second
	}
	if date, err := http.ParseTime(value); err == nil && time.Until(date) > 0 {
		return min(time.Until(date), maxRetryAfter)
	}
	return 0
}

// streamIdle is how long an execution stream may stay silent before the
// upload is abandoned, so a stalled stream does not hold the upload slot.
var streamIdle = 30 * time.Minute

func New(base, token string) (*Client, error) {
	v, err := Validate(models.UploadSettings{Enabled: true, URL: base, Token: token})
	if err != nil {
		return nil, err
	}
	return &Client{v.URL, v.Token, &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, ResponseHeaderTimeout: 30 * time.Second, DisableKeepAlives: true}, CheckRedirect: httpx.NoRedirect}}, nil
}

func (c *Client) request(ctx context.Context, method, endpoint string, data any) (*http.Response, error) {
	var b []byte
	if data != nil {
		b, _ = json.Marshal(data)
	}
	r, err := http.NewRequestWithContext(ctx, method, c.base+endpoint, bytes.NewReader(b))
	if err != nil {
		return nil, errors.New("invalid Upload-Assistant request")
	}
	r.Header.Set("Authorization", "Bearer "+c.token)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "text/event-stream, application/json")
	res, err := c.http.Do(r)
	if err != nil {
		return nil, httpx.RequestError(ctx, errors.New("Upload-Assistant connection failed or timed out"))
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		httpx.Close(res.Body)
		if res.StatusCode == http.StatusTooManyRequests {
			return nil, &RateLimitError{RetryAfter: retryAfter(res.Header.Get("Retry-After"))}
		}
		if httpx.AuthRejected(res.StatusCode) {
			return nil, errors.New("Upload-Assistant rejected the API token")
		}
		return nil, errors.New("Upload-Assistant returned an error response")
	}
	return res, nil
}

func (c *Client) Test(ctx context.Context) error {
	r, err := c.request(ctx, "GET", "/api/browse_roots", nil)
	if err != nil {
		return err
	}
	defer httpx.Close(r.Body)
	var out struct {
		Success bool `json:"success"`
	}
	if httpx.DecodeJSON(r.Body, 1<<20, &out) != nil || !out.Success {
		return errors.New("Upload-Assistant did not confirm API access")
	}
	return nil
}

// Outcome is how an execution ended: UA's exit code and, when its run events
// reported one, the target tracker's final result.
type Outcome struct {
	Code    int
	Tracker *models.TrackerResult
}

// stageMessages are the fixed progress messages for UA's run stages.
var stageMessages = map[string]string{
	"prep":     "Upload-Assistant is preparing the release",
	"confirm":  "Upload-Assistant is confirming the release details",
	"dupes":    "Upload-Assistant is checking for duplicates",
	"screens":  "Upload-Assistant is preparing screenshots",
	"torrent":  "Upload-Assistant is creating the torrent",
	"upload":   "Upload-Assistant is uploading to the tracker",
	"requests": "Upload-Assistant is checking tracker requests",
}

// finalTrackerStatuses are the tracker results that end a run; pending is not one.
var finalTrackerStatuses = []string{"success", "failed", "skipped", "dupe", "banned"}

func (c *Client) Execute(ctx context.Context, j models.UploadJob, progress func(string) error) (Outcome, error) {
	var out Outcome
	if !trackerPattern.MatchString(j.Tracker) {
		return out, errors.New("invalid tracker code")
	}
	args := "--trackers " + j.Tracker + " --unattended --skip-dupe-asking"
	extra, err := NormalizeArgs(j.Args)
	if err != nil {
		return out, err
	}
	if extra != "" {
		args += " " + extra
	}
	if j.Media.TMDbID > 0 && (j.Media.Type == "tv" || j.Media.Type == "movie") {
		args += fmt.Sprintf(" --tmdb %s/%d", j.Media.Type, j.Media.TMDbID)
	} else if imdb := releases.NormalizeIMDbID(j.Media.IMDbID); imdb != "" {
		args += " --imdb " + imdb
	}
	session := j.ID
	if j.Attempt > 0 {
		session += fmt.Sprintf("_retry_%d", j.Attempt)
	}
	// A watchdog cancels the request when the stream stays silent too long.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var stalled atomic.Bool
	idle := time.AfterFunc(streamIdle, func() {
		stalled.Store(true)
		cancel()
	})
	defer idle.Stop()
	r, err := c.request(ctx, "POST", "/api/execute", map[string]string{"path": j.UploadPath, "args": args, "session_id": session})
	if err != nil {
		return out, err
	}
	defer r.Body.Close()
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "text/event-stream") {
		return out, errors.New("Upload-Assistant did not return an execution stream")
	}
	scanner := bufio.NewScanner(r.Body)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	// current is the latest stage message, shown again when a prompt closes.
	// Once UA sends a known run stage, its generic progress events are ignored.
	data, shown, current, waiting, staged := "", "", "", false, false
	// report passes each changed progress message on once; the caller saves
	// every one, so the latest message is never lost.
	report := func(message string) error {
		if message == shown {
			return nil
		}
		shown = message
		return progress(message)
	}
	// stage reports a new stage, which also means no prompt is open.
	stage := func(message string) error {
		current, waiting = message, false
		return report(message)
	}
	handle := func() (*int, error) {
		if data == "" {
			return nil, nil
		}
		var event struct {
			Type string          `json:"type"`
			Code *int            `json:"code"`
			Data json.RawMessage `json:"data"`
		}
		if json.Unmarshal([]byte(data), &event) != nil {
			return nil, errors.New("invalid Upload-Assistant event")
		}
		data = ""
		switch event.Type {
		case "exit":
			if event.Code == nil {
				return nil, errors.New("missing Upload-Assistant exit code")
			}
			return event.Code, nil
		case "error":
			return nil, errors.New("Upload-Assistant reported an execution error")
		case "progress":
			var p struct {
				Group string `json:"group"`
			}
			_ = json.Unmarshal(event.Data, &p)
			// Run stages, when UA sends them, are more precise.
			if staged {
				return nil, nil
			}
			message := "Upload-Assistant is processing the release"
			if p.Group == "screenshots" {
				message = "Upload-Assistant is preparing screenshots"
			}
			if waiting {
				current = message // shown when the prompt closes
				return nil, nil
			}
			if err := stage(message); err != nil {
				return nil, err
			}
		case "gui":
			// Structured run events; older UA versions do not send them.
			var g struct {
				Kind      string          `json:"kind"`
				Key       string          `json:"key"`
				Status    string          `json:"status"`
				Action    string          `json:"action"`
				Tracker   string          `json:"tracker"`
				Message   string          `json:"message"`
				Link      string          `json:"link"`
				TorrentID json.RawMessage `json:"torrent_id"`
			}
			if json.Unmarshal(event.Data, &g) != nil {
				return nil, nil
			}
			switch g.Kind {
			case "stage":
				message, known := stageMessages[g.Key]
				staged = staged || known
				if known && g.Status == "start" {
					if err := stage(message); err != nil {
						return nil, err
					}
				}
			case "prompt":
				message := current
				if waiting = g.Action == "open"; waiting {
					message = "Upload-Assistant is waiting for an answer; check its WebUI"
				} else if message == "" {
					message = "Upload-Assistant is processing the release"
				}
				if err := report(message); err != nil {
					return nil, err
				}
			case "tracker":
				if strings.EqualFold(g.Tracker, j.Tracker) && slices.Contains(finalTrackerStatuses, g.Status) {
					out.Tracker = &models.TrackerResult{Status: g.Status, Message: plainText(g.Message, 500), Link: safeLink(g.Link), TorrentID: torrentID(g.TorrentID)}
				}
			}
		}
		return nil, nil
	}
	for scanner.Scan() {
		idle.Reset(streamIdle)
		line := scanner.Text()
		if line == "" {
			code, err := handle()
			if err != nil {
				return out, err
			}
			if code != nil {
				out.Code = *code
				return out, nil
			}
		} else if strings.HasPrefix(line, "data:") {
			data += strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " ") + "\n"
			if len(data) > 1<<20 {
				return out, errors.New("oversized Upload-Assistant event")
			}
		}
	}
	if err := scanner.Err(); err != nil {
		if idle.Stop(); stalled.Load() && waiting {
			return out, fmt.Errorf("Upload-Assistant waited %s for an answer; the upload was abandoned", streamIdle)
		} else if stalled.Load() {
			return out, fmt.Errorf("Upload-Assistant sent nothing for %s; the upload was abandoned", streamIdle)
		}
		if errors.Is(err, bufio.ErrTooLong) {
			return out, errors.New("oversized Upload-Assistant event")
		}
		return out, httpx.RequestError(ctx, errors.New("Upload-Assistant stream failed before completion"))
	}
	code, err := handle()
	if err != nil {
		return out, err
	}
	if code != nil {
		out.Code = *code
		return out, nil
	}
	return out, errors.New("Upload-Assistant stream ended without a completion event")
}

// torrentID reads a torrent ID sent as a string or as a number, keeping a
// large number's digits instead of a rounded float.
func torrentID(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return plainText(s, 64)
	}
	var n json.Number
	if json.Unmarshal(raw, &n) == nil {
		return plainText(n.String(), 64)
	}
	return ""
}

// ansiEscape matches terminal escape sequences: CSI, OSC and two-byte ones,
// which include an OSC's ESC \ terminator.
var ansiEscape = regexp.MustCompile(`\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\x07\x1b]*\x07?|[@-_])`)

// plainText keeps UA's already redacted text to one bounded line.
func plainText(value string, limit int) string {
	value = ansiEscape.ReplaceAllString(value, "")
	value = strings.Join(strings.FieldsFunc(value, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }), " ")
	if runes := []rune(value); len(runes) > limit {
		value = string(runes[:limit-1]) + "…"
	}
	return value
}

// safeLink keeps a tracker link only when it is plain http(s) without
// credentials, in case UA's redaction missed one. A tracker result links a
// torrent page, so download and feed links are dropped too.
func safeLink(value string) string {
	// Kept links are returned unescaped, so &amp; reaches the tracker as &.
	value = html.UnescapeString(strings.TrimSpace(value))
	u, err := url.Parse(value)
	if err != nil || len(value) > 2000 || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return ""
	}
	if models.DownloadPath(u.EscapedPath()) || models.HasCredential(value) {
		return ""
	}
	return u.String()
}
