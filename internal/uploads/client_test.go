package uploads

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"ravenseed/internal/models"
	"strings"
	"testing"
	"time"
)

func TestPaths(t *testing.T) {
	v, err := Validate(models.UploadSettings{Paths: []models.UploadPath{{InstanceID: 1, From: "/downloads", To: "/data"}, {InstanceID: 1, From: "/downloads/movies", To: "/movies"}, {InstanceID: 2, From: `D:\Downloads`, To: "/windows"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		from, want string
		instance   int
	}{{"/downloads/movies/Test/film.mkv", "/movies/Test/film.mkv", 1}, {`d:\Downloads\Movie.mkv`, "/windows/Movie.mkv", 2}} {
		got, err := MapPath(tc.from, tc.instance, v.Paths)
		if err != nil || got != tc.want {
			t.Fatalf("mapping: %s %v", got, err)
		}
	}
	for _, input := range []string{"/downloads-other/movie", "/downloads/../private/file", "relative/file"} {
		if _, err := MapPath(input, 1, v.Paths); err == nil {
			t.Fatalf("unsafe mapping accepted: %s", input)
		}
	}
	if _, err := MapPath("/downloads/movie", 3, v.Paths); err == nil {
		t.Fatal("wrong instance mapped")
	}
}

func TestExecutionStream(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		fail       bool
	}{
		{"exit", "data: {\"type\":\"progress\",\"data\":{\"group\":\"screenshots\"}}\n\ndata: {\"type\":\"exit\",\"code\":0}\n\n", false},
		{"disconnect", "data: {\"type\":\"html\",\"data\":\"private-token\"}\n\n", true},
		{"error", "data: {\"type\":\"error\",\"data\":\"private-token\"}\n\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer private-token" || r.URL.Path != "/ua/api/execute" {
					t.Error("wrong authentication or endpoint")
				}
				var body map[string]string
				_ = json.NewDecoder(r.Body).Decode(&body)
				if body["path"] != "/data/movie" || body["session_id"] != "job1" || body["args"] != "--trackers AITHER --unattended --skip-dupe-asking --tmdb movie/603" {
					t.Error("incorrect upload scope")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			c, _ := New(server.URL+"/ua", "private-token")
			_, err := c.Execute(context.Background(), models.UploadJob{ID: "job1", Tracker: "AITHER", UploadPath: "/data/movie", Media: models.Media{Type: "movie", TMDbID: 603}}, func(s string) error {
				if strings.Contains(s, "private-token") {
					t.Error("raw output leaked")
				}
				return nil
			})
			if (err != nil) != tc.fail {
				t.Fatalf("execution: %v", err)
			}
			if err != nil && strings.Contains(err.Error(), "private-token") {
				t.Fatal("secret exposed")
			}
		})
	}
}

func TestExecutionTrackerResult(t *testing.T) {
	gui := func(data string) string { return `data: {"type":"gui","data":` + data + "}\n\n" }
	body := gui(`{"kind":"stage","key":"dupes","status":"start"}`) +
		"data: {\"type\":\"progress\",\"data\":{\"group\":\"dupes\"}}\n\n" +
		gui(`{"kind":"stage","key":"dupes","status":"done"}`) +
		gui(`{"kind":"tracker","tracker":"ULCX","status":"success"}`) +
		gui(`{"kind":"tracker","tracker":"aither","status":"pending"}`) +
		gui(`{"kind":"tracker","tracker":"AITHER","status":"skipped","message":"Check declined:\n\u001b[31mno English audio\u0007","link":"https://aither.example/download?id=1&passkey=private-token","torrent_id":42}`) +
		gui(`{"kind":"stage","key":"upload","status":"start"}`) +
		"data: {\"type\":\"exit\",\"code\":0}\n\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()
	c, _ := New(server.URL, "private-token")
	var messages []string
	out, err := c.Execute(context.Background(), models.UploadJob{ID: "job1", Tracker: "AITHER", UploadPath: "/data/movie"}, func(s string) error {
		messages = append(messages, s)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := models.TrackerResult{Status: "skipped", Message: "Check declined: no English audio", TorrentID: "42"}
	if out.Code != 0 || out.Tracker == nil || *out.Tracker != want {
		t.Fatalf("tracker result: %+v", out.Tracker)
	}
	if strings.Join(messages, "|") != "Upload-Assistant is checking for duplicates|Upload-Assistant is uploading to the tracker" {
		t.Fatalf("progress: %q", messages)
	}
}

func TestExecutionPromptAndUnknownEvents(t *testing.T) {
	gui := func(data string) string { return `data: {"type":"gui","data":` + data + "}\n\n" }
	progress := "data: {\"type\":\"progress\",\"data\":{\"group\":\"other\"}}\n\n"
	for name, test := range map[string]struct{ body, want string }{
		// A closed prompt shows the stage it interrupted again.
		"prompt closes": {gui(`{"kind":"stage","key":"dupes","status":"start"}`) + gui(`{"kind":"prompt","action":"open"}`) + gui(`{"kind":"prompt","action":"close"}`),
			"Upload-Assistant is checking for duplicates|Upload-Assistant is waiting for an answer; check its WebUI|Upload-Assistant is checking for duplicates"},
		// Generic progress counts until a known stage arrives.
		"unknown event": {gui(`{"kind":"banner","text":"hello"}`) + gui(`{"kind":"stage","key":"later","status":"start"}`) + progress + gui(`{"kind":"stage","key":"upload","status":"start"}`) + progress,
			"Upload-Assistant is processing the release|Upload-Assistant is uploading to the tracker"},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte(test.body + "data: {\"type\":\"exit\",\"code\":0}\n\n"))
		}))
		c, _ := New(server.URL, "private-token")
		var messages []string
		_, err := c.Execute(context.Background(), models.UploadJob{ID: "job1", Tracker: "AITHER", UploadPath: "/data/movie"}, func(s string) error {
			messages = append(messages, s)
			return nil
		})
		server.Close()
		if err != nil || strings.Join(messages, "|") != test.want {
			t.Errorf("%s: %q %v", name, messages, err)
		}
	}
}

func TestTrackerResultText(t *testing.T) {
	for raw, want := range map[string]string{`12345678901234567`: "12345678901234567", `"12345678901234567"`: "12345678901234567", `9`: "9", `null`: "", `true`: "", ``: ""} {
		if got := torrentID(json.RawMessage(raw)); got != want {
			t.Errorf("torrentID(%s) = %q, want %q", raw, got, want)
		}
	}
	for value, want := range map[string]string{
		"\x1b[1;31mred\x1b[0m text":                       "red text",
		"\x1b]8;;https://x.example\x07link\x1b]8;;\x1b\\": "link",
		"bell\x07 and\ttab":                               "bell and tab",
	} {
		if got := plainText(value, 500); got != want {
			t.Errorf("plainText(%q) = %q, want %q", value, got, want)
		}
	}
}

func TestSafeLink(t *testing.T) {
	hash := "0123456789abcdef0123456789abcdef01234567"
	for link, want := range map[string]string{
		"https://aither.example/torrents/9":                                           "https://aither.example/torrents/9",
		"https://tracker.example/torrents.php?id=1":                                   "https://tracker.example/torrents.php?id=1",
		"https://tracker.example/torrents/" + hash:                                    "https://tracker.example/torrents/" + hash,
		"https://tracker.example/torrents/TheMatrixReloaded2003BluRay1080pRemux":      "https://tracker.example/torrents/TheMatrixReloaded2003BluRay1080pRemux",
		"https://tracker.example/details.php?id=1&designation=movie&author=someone":   "https://tracker.example/details.php?id=1&designation=movie&author=someone",
		"https://tracker.example/torrents/9?keyword=matrix&monkey=1":                  "https://tracker.example/torrents/9?keyword=matrix&monkey=1",
		"https://tracker.example/dl.php?id=1&torrent_pass=x":                          "",
		"https://tracker.example/torrents/9?passkey=abc123":                           "",
		"https://tracker.example/torrents/9?torrentPass=abc":                          "",
		"https://tracker.example/torrents/9?apiKey=abc":                               "",
		"https://tracker.example/torrents/9?api_token=abc":                            "",
		"https://tracker.example/torrents/9?sig=abc":                                  "",
		"https://tracker.example/torrents/9?pk=abc":                                   "",
		"https://tracker.example/torrents/9?uid=1&rsskey=abc":                         "",
		"https://tracker.example/torrents/9#authkey=abc":                              "",
		"https://tracker.example/torrents/9?id=1&amp;passkey=abc":                     "",
		"https://tracker.example/torrents/9?id=1;passkey=abc":                         "",
		"https://tracker.example/torrents/9?pass%6Bey=abc":                            "",
		"https://tracker.example/torrents/9?%70asskey=abc":                            "",
		"https://tracker.example/torrents.php?id=1&amp;torrentid=2":                   "https://tracker.example/torrents.php?id=1&torrentid=2",
		"https://aither.example/torrents/download/9.0123456789abcdef0123456789abcdef": "",
		"https://tracker.example/download.php?id=1":                                   "",
		"https://tracker.example/dl/9":                                                "",
		"https://tracker.example/rss/0123456789abcdef0123456789abcdef/9":              "",
		"https://tracker.example/torrents/0123456789abcdef0123456789abcdef":           "",
		"https://tracker.example/torrents/9?id=0123456789abcdef0123456789abcdef":      "",
		"https://tracker.example/torrents/9#0123456789abcdef0123456789abcdef":         "",
		"https://user:pass@tracker.example/torrents/9":                                "",

		"javascript:alert(1)": "",
		"/torrents/9":         "",
	} {
		if got := safeLink(link); got != want {
			t.Errorf("safeLink(%q) = %q", link, got)
		}
	}
}

func TestStallWhileWaitingForAnswer(t *testing.T) {
	defer func(d time.Duration) { streamIdle = d }(streamIdle)
	streamIdle = 50 * time.Millisecond
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"type":"gui","data":{"kind":"prompt","action":"open","id":"p1","type":"yes_no","question":"Upload anyway?"}}` + "\n\n"))
		w.(http.Flusher).Flush()
		<-release
	}))
	defer server.Close()
	defer close(release)
	c, _ := New(server.URL, "private-token")
	_, err := c.Execute(context.Background(), models.UploadJob{ID: "job1", Tracker: "AITHER", UploadPath: "/data/movie"}, func(string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "for an answer") {
		t.Fatalf("stall: %v", err)
	}
}

func TestRejectsRedirectAndInvalidTracker(t *testing.T) {
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("redirect followed") }))
	defer receiver.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, receiver.URL, 302) }))
	defer server.Close()
	c, _ := New(server.URL, "private-token")
	if err := c.Test(context.Background()); err == nil {
		t.Fatal("redirect accepted")
	}
	if _, err := Validate(models.UploadSettings{Trackers: []models.UploadTracker{{IndexerID: 1, Code: "AITHER --skip-dupe-check"}}}); err == nil {
		t.Fatal("tracker arguments accepted")
	}
}
