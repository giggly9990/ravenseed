package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

// writeJSON sends v as a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// categories lists the qui categories: the two the add-torrent demo offers
// plus every category a seeded inventory torrent uses.
func categories() map[string]any {
	out := map[string]any{
		"Movies": map[string]string{"name": "Movies", "savePath": "/downloads/movies"},
		"TV":     map[string]string{"name": "TV", "savePath": "/downloads/tv"},
	}
	for _, t := range inventory {
		if _, ok := out[t.category]; !ok && t.category != "" {
			out[t.category] = map[string]string{"name": t.category, "savePath": "/downloads/" + t.category}
		}
	}
	return out
}

// maxDownloadTags bounds the tags remembered from added torrents, so a public
// demo cannot grow the mock's memory without limit.
const maxDownloadTags = 256

// downloadedHash is the hash every torrent added through the mock reports.
var downloadedHash = strings.Repeat("b", 40)

var (
	downloadMu   sync.Mutex
	downloadTags []string
)

// rememberDownload records the tags of an added torrent, keeping the newest
// maxDownloadTags entries.
func rememberDownload(tags string) {
	if tags == "" {
		return
	}
	downloadMu.Lock()
	defer downloadMu.Unlock()
	downloadTags = append(downloadTags, tags)
	if n := len(downloadTags) - maxDownloadTags; n > 0 {
		downloadTags = slices.Delete(downloadTags, 0, n)
	}
}

// hasDownloads reports whether any tagged torrent was added.
func hasDownloads() bool {
	downloadMu.Lock()
	defer downloadMu.Unlock()
	return len(downloadTags) > 0
}

// mockDownloads answers qui's torrent list, filtered by hashes or one tag.
func mockDownloads(w http.ResponseWriter, r *http.Request) {
	var filter struct {
		Tags   []string `json:"tags"`
		Hashes []string `json:"hashes"`
	}
	_ = json.Unmarshal([]byte(r.URL.Query().Get("filters")), &filter)
	wanted := func(hash string) bool {
		return len(filter.Hashes) == 0 || slices.ContainsFunc(filter.Hashes, func(h string) bool { return strings.EqualFold(h, hash) })
	}
	items := []any{}
	if len(filter.Tags) == 0 {
		for _, t := range inventory {
			if wanted(t.hash) {
				items = append(items, map[string]any{"hash": t.hash, "name": t.name, "tracker": t.tracker, "category": t.category, "total_size": t.size(), "state": "stalledUP", "progress": 1, "amount_left": 0, "content_path": t.path})
			}
		}
	}
	if len(filter.Tags) == 1 && filter.Tags[0] != "" && wanted(downloadedHash) {
		downloadMu.Lock()
		for _, tag := range downloadTags {
			if strings.Contains(tag, filter.Tags[0]) {
				items = append(items, map[string]any{"hash": downloadedHash, "tags": tag, "state": "stalledUP", "progress": 1, "amount_left": 0, "content_path": "/downloads/Demo/movie.mkv"})
			}
		}
		downloadMu.Unlock()
	}
	writeJSON(w, 200, map[string]any{"torrents": items})
}

func main() {
	metadataRoutes()
	uploadRoutes()
	arrRoutes()
	http.HandleFunc("/api/instances", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "demo-key" {
			w.WriteHeader(401)
			return
		}
		writeJSON(w, 200, []any{map[string]any{"id": 1, "name": "Demo qBittorrent (mock)", "connected": true, "isActive": true}})
	})
	http.HandleFunc("/api/instances/1/categories", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "demo-key" {
			w.WriteHeader(401)
			return
		}
		writeJSON(w, 200, categories())
	})
	http.HandleFunc("/api/instances/1/torrents", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "demo-key" {
			w.WriteHeader(401)
			return
		}
		if r.Method == "GET" {
			mockDownloads(w, r)
			return
		}
		if r.Method != "POST" || r.ParseMultipartForm(1<<20) != nil || r.FormValue("urls") == "" || r.FormValue("indexer_id") == "" {
			w.WriteHeader(400)
			return
		}
		rememberDownload(r.FormValue("tags"))
		writeJSON(w, 201, map[string]any{"added": 1, "failed": 0})
	})
	http.HandleFunc("/api/torznab/indexers", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "demo-key" {
			w.WriteHeader(401)
			return
		}
		writeJSON(w, 200, []any{map[string]any{"id": 1, "name": "Demo Atlas", "enabled": true, "backend": "prowlarr", "capabilities": []string{"movie-search", "movie-search-imdbid", "movie-search-tmdbid", "tv-search", "tv-search-imdbid", "tv-search-tmdbid", "tv-search-season", "tv-search-ep"}}, map[string]any{"id": 2, "name": "Demo Orbit", "enabled": true, "backend": "jackett"}, map[string]any{"id": 3, "name": "Demo Beacon", "enabled": true, "backend": "native"}})
	})
	http.HandleFunc("/api/torznab/search", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "demo-key" {
			w.WriteHeader(401)
			return
		}
		var q struct {
			IndexerIDs []int  `json:"indexer_ids"`
			IMDbID     string `json:"imdb_id"`
			TMDbID     int    `json:"tmdb_id"`
			TVDbID     string `json:"tvdb_id"`
			Query      string `json:"query"`
			Season     *int   `json:"season"`
			Episode    *int   `json:"episode"`
		}
		if json.NewDecoder(r.Body).Decode(&q) != nil || len(q.IndexerIDs) != 1 {
			w.WriteHeader(400)
			return
		}
		select {
		case <-time.After(500 * time.Millisecond):
		case <-r.Context().Done():
			return
		}
		id := q.IndexerIDs[0]
		rows := []any{}
		if fixtures, ok := fixtureListings(q.IMDbID, q.TMDbID, q.TVDbID, q.Query, id); ok && q.Season == nil {
			for _, v := range fixtures {
				rows = append(rows, map[string]any{"indexer_id": id, "title": v.title, "size": v.size, "seeders": v.seeders, "search_imdb_id": q.IMDbID, "search_tmdb_id": q.TMDbID, "search_tvdb_id": q.TVDbID, "publish_date": "2026-09-20T12:00:00Z", "download_volume_factor": 1, "upload_volume_factor": 1, "download_url": "https://tracker.example/demo.torrent", "info_url": "https://tracker.example/details/" + v.title, "guid": v.title})
			}
			writeResults(w, rows)
			return
		}
		if q.IMDbID == "tt9999999" || q.TMDbID == 99999 {
			if id != 3 {
				title := "The.Matrix.1999.CUSTOMTAG.1080p.NF.WEB-DL.H.264-OldGroup"
				if id == 2 {
					title = "La.Matrice.1999.1080p.NETFLIX.WEB.DL.H.264-GROUP [tracker.example]"
				}
				if q.TMDbID == 99999 {
					title = "[SubsPlease] Example Show - 13 (1080p WEB-DL HEVC) [AB12CD34].mkv"
					if id == 2 {
						title = "[SubsPlease] Example Show S02E01 (1080p WEB-DL HEVC)"
					}
				}
				rows = append(rows, map[string]any{"indexer_id": id, "title": title, "size": 10000000, "seeders": 8, "search_imdb_id": q.IMDbID, "search_tmdb_id": q.TMDbID, "guid": title, "publish_date": "2026-09-20T12:00:00Z"})
			}
			writeResults(w, rows)
			return
		}
		title := "The.Matrix.1999"
		if q.Season != nil {
			title = fmt.Sprintf("Example.Show.S%02d", *q.Season)
		}
		if q.Episode != nil {
			title += fmt.Sprintf("E%02d", *q.Episode)
		}
		add := func(suffix string, size int64, seeds int) {
			factor := 1.0
			if id == 1 {
				factor = 0
			}
			if id == 2 {
				factor = 0.5
			}
			rows = append(rows, map[string]any{"indexer_id": id, "title": title + suffix, "size": size, "seeders": seeds, "search_imdb_id": q.IMDbID, "search_tmdb_id": q.TMDbID, "search_tvdb_id": q.TVDbID, "publish_date": "2026-09-20T12:00:00Z", "download_volume_factor": factor, "upload_volume_factor": 2, "download_url": "https://tracker.example/demo.torrent", "guid": suffix})
		}
		if id != 3 {
			add(".2160p.UHD.BluRay.REMUX.HEVC.DV.HDR.TrueHD.7.1.Atmos-FraMeSToR", 82123456789, 21-id)
			add(".1080p.BluRay.x264.DTS-GROUP", 12884901888, 35-id)
		}
		add(".1080p.WEB-DL.DDP5.1.H.264-NTb", 6432187654, 12+id)
		if id == 2 {
			add(".2160p.UHD.BluRay.REMUX.HEVC.DV.HDR.TrueHD.7.1.Atmos-FraMeSToR", 82123456889, 4)
		}
		writeResults(w, rows)
	})
	server := &http.Server{Addr: ":7476", ReadHeaderTimeout: 5 * time.Second}
	if err := server.ListenAndServe(); err != nil {
		panic(err)
	}
}

// writeResults sends a torznab search response.
func writeResults(w http.ResponseWriter, rows []any) {
	writeJSON(w, 200, map[string]any{"results": rows, "total": len(rows)})
}
