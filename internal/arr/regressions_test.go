package arr

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ravenseed/internal/models"
)

func TestValidateRejectsDuplicateNames(t *testing.T) {
	_, err := Validate(models.ArrSettings{Instances: []models.ArrInstance{
		{Name: "Main", Kind: Sonarr, URL: "http://sonarr:8989"},
		{Name: "main ", Kind: Radarr, URL: "http://radarr:7878"},
	}})
	if err == nil {
		t.Fatal("duplicate instance names accepted")
	}
}

func TestIMDbNormalization(t *testing.T) {
	c := &Client{kind: Radarr}
	for in, want := range map[string]string{"tt0133093": "tt0133093", "0133093": "tt0133093", "tt0": "", "tt-5": "", "tt+5": "", "": ""} {
		if got := c.media(item{IMDbID: in}).IMDbID; got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
	if m := c.media(item{TMDbID: -3}); m.TMDbID != 0 || hasID(m) {
		t.Fatalf("negative TMDb ID kept: %+v", m)
	}
}

func TestTestDoesNotEchoTheRemoteApp(t *testing.T) {
	s := server(t, map[string]string{"/api/v3/system/status": `{"appName":"Secret Internal Service"}`})
	c, _ := New(s.URL, "key", Sonarr, time.Second)
	err := c.Test(context.Background())
	if err == nil || strings.Contains(err.Error(), "Secret") {
		t.Fatalf("remote app name echoed: %v", err)
	}
	bad, _ := New(s.URL, "wrong", Sonarr, time.Second)
	if err := bad.Test(context.Background()); err == nil || !strings.Contains(err.Error(), "rejected the API key") {
		t.Fatalf("auth failure: %v", err)
	}
}

func TestRedirectsAreNotFollowed(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://example.invalid/steal", http.StatusFound)
	}))
	t.Cleanup(s.Close)
	c, _ := New(s.URL, "key", Radarr, time.Second)
	if err := c.Test(context.Background()); err == nil {
		t.Fatal("redirect followed or accepted")
	}
}

func TestHistoryPagesAndDeduplicatesEpisodes(t *testing.T) {
	hash := strings.Repeat("ab", 20)
	pages := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/history" {
			w.WriteHeader(404)
			return
		}
		pages++
		records := []string{}
		page := r.URL.Query().Get("page")
		count := historyPageSize
		if page == "2" {
			count = 2
		}
		for i := 0; i < count; i++ {
			// Each episode appears for its grab and its import.
			episode := (i/2)%10 + 1
			if page == "2" {
				episode = 11
			}
			records = append(records, fmt.Sprintf(`{"downloadId":%q,"seriesId":5,"series":{"id":5,"title":"Show","tvdbId":1,"tmdbId":2},"episode":{"seasonNumber":1,"episodeNumber":%d,"absoluteEpisodeNumber":%d}}`, hash, episode, episode))
		}
		fmt.Fprintf(w, `{"totalRecords":%d,"records":[%s]}`, historyPageSize+2, strings.Join(records, ","))
	}))
	t.Cleanup(s.Close)
	c, _ := New(s.URL, "key", Sonarr, time.Second)
	m, err := c.History(context.Background(), hash)
	if err != nil || m == nil {
		t.Fatalf("history: %+v %v", m, err)
	}
	if pages != 2 || len(m.Episodes) != 11 {
		t.Fatalf("pages %d, episodes %d: %+v", pages, len(m.Episodes), m.Episodes)
	}
}

func TestHistoryAmbiguousOwner(t *testing.T) {
	hash := strings.Repeat("ab", 20)
	s := server(t, map[string]string{"/api/v3/history": `{"records":[
		{"downloadId":"` + hash + `","movieId":3,"movie":{"title":"A","tmdbId":1}},
		{"downloadId":"` + hash + `","movieId":4,"movie":{"title":"B","tmdbId":2}}]}`})
	c, _ := New(s.URL, "key", Radarr, time.Second)
	if m, err := c.History(context.Background(), hash); m != nil || err != nil {
		t.Fatalf("ambiguous owner matched: %+v %v", m, err)
	}
}

func TestImportsKeepNewestRecordsPastTheSizeCap(t *testing.T) {
	record := `{"eventType":"downloadFolderImported","downloadId":"AB","data":{"droppedPath":"/d/Movie.2013.1080p.BluRay.x264-GRP.mkv","size":"1000"}}`
	body := "[" + strings.TrimSuffix(strings.Repeat(record+",", 5), ",") + "]"
	files, err := decodeArrayPrefix[importRecord](strings.NewReader(body), int64(len(record)*3))
	if err != nil || len(files) < 2 || len(files) > 3 {
		t.Fatalf("cut import history: %d %v", len(files), err)
	}
	if _, err := decodeArrayPrefix[importRecord](strings.NewReader(`{"not":"array"}`), 1<<20); err == nil {
		t.Fatal("non-array accepted")
	}
	if all, err := decodeArrayPrefix[importRecord](strings.NewReader(body), 1<<20); err != nil || len(all) != 5 {
		t.Fatalf("full import history: %d %v", len(all), err)
	}
	s := server(t, map[string]string{"/api/v3/history/movie": body})
	c, _ := New(s.URL, "key", Radarr, time.Second)
	imports, err := c.Imports(context.Background(), 3)
	if err != nil || len(imports) != 5 || imports[0].Name != "Movie.2013.1080p.BluRay.x264-GRP.mkv" || imports[0].DownloadID != "ab" {
		t.Fatalf("radarr imports: %+v %v", imports, err)
	}
}
