package arr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"ravenseed/internal/models"
	"strings"
	"testing"
	"time"
)

func server(t *testing.T, routes map[string]string) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "key" {
			w.WriteHeader(401)
			return
		}
		body, ok := routes[r.URL.Path]
		if !ok {
			w.WriteHeader(404)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(s.Close)
	return s
}

func TestRadarrHistoryFiltersByHash(t *testing.T) {
	hash := strings.Repeat("ab", 20)
	s := server(t, map[string]string{
		"/api/v3/history": `{"records":[
			{"downloadId":"OTHER","movieId":9,"movie":{"title":"Wrong","tmdbId":1}},
			{"downloadId":"` + strings.ToUpper(hash) + `","movieId":3,"movie":{"title":"The Matrix","year":1999,"tmdbId":603,"imdbId":"tt0133093"}}]}`,
	})
	c, err := New(s.URL, "key", Radarr, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	m, err := c.History(context.Background(), hash)
	if err != nil || m == nil {
		t.Fatalf("history: %+v %v", m, err)
	}
	if m.Media.Type != "movie" || m.Media.TMDbID != 603 || m.Media.IMDbID != "tt0133093" || m.Media.TVDbID != 0 {
		t.Fatalf("media: %+v", m.Media)
	}
	if m, err = c.History(context.Background(), strings.Repeat("cd", 20)); m != nil || err != nil {
		t.Fatalf("unrelated hash matched: %+v %v", m, err)
	}
}

func TestSonarrHistoryFetchesSeriesAndAbsoluteEpisodes(t *testing.T) {
	hash := strings.Repeat("ef", 20)
	s := server(t, map[string]string{
		"/api/v3/history":       `{"records":[{"downloadId":"` + hash + `","seriesId":5,"quality":{"quality":{"name":"WEBDL-1080p","resolution":1080}},"episode":{"seasonNumber":4,"episodeNumber":15,"absoluteEpisodeNumber":81}}]}`,
		"/api/v3/series/5":      `{"id":5,"title":"Attack on Titan","year":2013,"tvdbId":267440,"imdbId":"tt2560140","tmdbId":1429,"runtime":24,"seasons":[{"seasonNumber":4,"statistics":{"totalEpisodeCount":30}},{"seasonNumber":0}]}`,
		"/api/v3/parse":         `{"series":null}`,
		"/api/v3/system/status": `{"appName":"Sonarr"}`,
	})
	c, _ := New(s.URL, "key", Sonarr, time.Second)
	m, err := c.History(context.Background(), hash)
	if err != nil || m == nil {
		t.Fatalf("history: %+v %v", m, err)
	}
	if m.Media.Type != "tv" || m.Media.TVDbID != 267440 || m.Media.IMDbID != "tt2560140" || m.Resolution != "1080p" || m.Runtime != 24 || len(m.SeasonEpisodes) != 1 || m.SeasonEpisodes[4] != 30 {
		t.Fatalf("media: %+v", m.Media)
	}
	if len(m.Episodes) != 1 || m.Episodes[0] != (models.EpisodeMapping{IMDbID: "tt2560140", TMDbID: 1429, Absolute: 81, Season: 4, Episode: 15}) {
		t.Fatalf("episodes: %+v", m.Episodes)
	}
	if err = c.Test(context.Background()); err != nil {
		t.Fatal(err)
	}
	r, _ := New(s.URL, "key", Radarr, time.Second)
	if err = r.Test(context.Background()); err == nil {
		t.Fatal("Sonarr accepted as Radarr")
	}
	if m, err = c.Parse(context.Background(), "Unknown.Show.S01E01"); m != nil || err != nil {
		t.Fatalf("unmatched parse: %+v %v", m, err)
	}
}

func TestLookupRequiresOneExactMatch(t *testing.T) {
	s := server(t, map[string]string{
		"/api/v3/movie/lookup": `[
			{"title":"Dune","year":2021,"tmdbId":438631,"imdbId":"tt1160419"},
			{"title":"Dune","year":1984,"tmdbId":841,"imdbId":"tt0087182"},
			{"title":"Dune: Part Two","year":2024,"tmdbId":693134}]`,
	})
	c, _ := New(s.URL, "key", Radarr, time.Second)
	m, err := c.Lookup(context.Background(), "Dune", 2021)
	if err != nil || m == nil || m.Media.TMDbID != 438631 {
		t.Fatalf("lookup: %+v %v", m, err)
	}
	for _, year := range []int{0, 2000} {
		if m, _ = c.Lookup(context.Background(), "Dune", year); m != nil {
			t.Fatalf("year %d resolved ambiguously: %+v", year, m)
		}
	}
}

func TestValidate(t *testing.T) {
	ok := models.ArrInstance{Name: "Sonarr", Kind: "Sonarr", URL: "http://sonarr:8989/", APIKey: "key", Enabled: true}
	v, err := Validate(models.ArrSettings{Instances: []models.ArrInstance{ok}})
	if err != nil || v.Instances[0].Kind != Sonarr || v.Instances[0].URL != "http://sonarr:8989" {
		t.Fatalf("%+v %v", v, err)
	}
	for _, bad := range []models.ArrInstance{
		{Name: "x", Kind: "lidarr", URL: "http://a", APIKey: "k"},
		{Name: "", Kind: Sonarr, URL: "http://a", APIKey: "k"},
		{Name: "x", Kind: Sonarr, URL: "ftp://a", APIKey: "k"},
		{Name: "x", Kind: Sonarr, URL: "http://u:p@a", APIKey: "k"},
		{Name: "x", Kind: Sonarr, URL: "http://a", Enabled: true},
	} {
		if _, err := Validate(models.ArrSettings{Instances: []models.ArrInstance{bad}}); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
}

func TestImportsReadDroppedFileAndSize(t *testing.T) {
	s := server(t, map[string]string{
		"/api/v3/history/series": `[
			{"eventType":"downloadFolderImported","downloadId":"A8CD","episode":{"seasonNumber":2,"episodeNumber":10,"absoluteEpisodeNumber":20},"quality":{"quality":{"resolution":1080}},
			 "data":{"droppedPath":"/tv_media/torrent/tv/Cyberpunk.Edgerunners.S02.1080p.NF.WEB-DL-FLUX/Cyberpunk.Edgerunners.S02E10.1080p.NF.WEB-DL-FLUX.mkv","size":"1367721339"}},
			{"eventType":"grabbed","downloadId":"A8CD","data":{"size":"999"}},
			{"eventType":"downloadFolderImported","downloadId":"B1","data":{"droppedPath":"C:\\Downloads\\Other.mkv","size":"not-a-number"}}]`,
	})
	c, _ := New(s.URL, "key", Sonarr, time.Second)
	got, err := c.Imports(context.Background(), 247)
	if err != nil || len(got) != 1 {
		t.Fatalf("imports: %+v %v", got, err)
	}
	want := ImportedFile{Name: "Cyberpunk.Edgerunners.S02E10.1080p.NF.WEB-DL-FLUX.mkv", Size: 1367721339, DownloadID: "a8cd", Resolution: "1080p"}
	if got[0].Name != want.Name || got[0].Size != want.Size || got[0].DownloadID != want.DownloadID || got[0].Resolution != want.Resolution || got[0].Episode == nil || *got[0].Episode != (models.EpisodeMapping{Season: 2, Episode: 10, Absolute: 20}) {
		t.Fatalf("import: %+v", got[0])
	}
}

func TestLibraryFilesCarryMeasuredVideoBitrate(t *testing.T) {
	s := server(t, map[string]string{
		"/api/v3/moviefile": `[
			{"size":18556478093,"originalFilePath":"Babylon.2022.BluRay.1080p.DDP.Atmos.5.1.x264-hallowed.mkv","mediaInfo":{"videoBitrate":11996159,"audioBitrate":1024000,"runTime":"3:08:55"}},
			{"size":5,"mediaInfo":null}]`,
	})
	c, _ := New(s.URL, "key", Radarr, time.Second)
	files, err := c.LibraryFiles(context.Background(), 1072)
	if err != nil || len(files) != 1 || files[0] != (LibraryFile{Size: 18556478093, VideoBitrate: 11996159}) {
		t.Fatalf("library files: %+v %v", files, err)
	}
}
