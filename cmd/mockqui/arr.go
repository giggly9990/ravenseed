package main

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Read-only Radarr and Sonarr endpoints RavenSeed uses to identify torrents.
func arrRoutes() {
	arrRoute("/radarr/api/v3/", "demo-radarr-key", radarr)
	arrRoute("/sonarr/api/v3/", "demo-sonarr-key", sonarr)
}

func arrRoute(prefix, key string, handle func(path string, q url.Values) (any, bool)) {
	http.HandleFunc(prefix, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != key {
			w.WriteHeader(401)
			return
		}
		if r.Method != "GET" {
			w.WriteHeader(405)
			return
		}
		out, ok := handle(strings.TrimPrefix(r.URL.Path, prefix), r.URL.Query())
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, 200, out)
	})
}

// importedEvent is the numeric history eventType filter for
// downloadFolderImported records.
const importedEvent = "3"

// eventAllowed reports whether a history record of the named event type
// passes the request's numeric eventType filter, if any.
func eventAllowed(q url.Values, event string) bool {
	filter := q.Get("eventType")
	return filter == "" || (filter == importedEvent && event == "downloadFolderImported") || (filter == "1" && event == "grabbed")
}

// include adds value under key when the request's include flag is "true",
// as Sonarr and Radarr only embed related resources on request.
func include(record map[string]any, q url.Values, flag, key string, value any) {
	if q.Get(flag) == "true" {
		record[key] = value
	}
}

func movieItem(m *movie) map[string]any {
	return map[string]any{"id": m.id, "title": m.title, "originalTitle": m.title, "year": m.year, "imdbId": m.imdb, "tmdbId": m.tmdb, "runtime": m.runtime, "alternateTitles": []any{}}
}

func quality(resolution int) map[string]any {
	return map[string]any{"quality": map[string]any{"resolution": resolution}}
}

// radarrGrabs maps torrent hashes Radarr downloaded to its movie and quality.
var radarrGrabs = map[string][2]int{matrixRemuxHash: {1, 2160}, arrivalSeedHash: {3, 1080}}

// radarrParse lists release names Radarr matches to a library movie.
var radarrParse = map[string]int{"The.Matrix.1999.1080p.WEB-DL.DDP5.1.H.264-NTb": 1, duneName: 2, arrivalName: 3}

var radarrFiles = map[int][2]int64{1: {82123456789, 58000000}, 2: {7516192768, 8200000}, 3: {11811160064, 12500000}}

func radarr(path string, q url.Values) (any, bool) {
	get := q.Get
	switch {
	case path == "system/status":
		return map[string]string{"appName": "Radarr", "version": "5.14.0-demo"}, true
	case path == "history":
		records := []any{}
		hash := strings.ToLower(get("downloadId"))
		if grab, ok := radarrGrabs[hash]; ok && eventAllowed(q, "grabbed") {
			record := map[string]any{"downloadId": strings.ToUpper(hash), "movieId": grab[0], "eventType": "grabbed", "quality": quality(grab[1])}
			include(record, q, "includeMovie", "movie", movieItem(movieByID(grab[0])))
			records = append(records, record)
		}
		return map[string]any{"page": 1, "pageSize": 100, "totalRecords": len(records), "records": records}, true
	case strings.HasPrefix(path, "movie/") && path != "movie/lookup":
		id, _ := strconv.Atoi(strings.TrimPrefix(path, "movie/"))
		if m := movieByID(id); m != nil {
			return movieItem(m), true
		}
		return nil, false
	case path == "parse":
		out := map[string]any{"title": get("title")}
		if id, ok := radarrParse[get("title")]; ok {
			out["movie"] = movieItem(movieByID(id))
		}
		return out, true
	case path == "movie/lookup":
		out := []any{}
		for i := range movies {
			if strings.EqualFold(movies[i].title, strings.TrimSpace(get("term"))) {
				out = append(out, movieItem(&movies[i]))
			}
		}
		return out, true
	case path == "history/movie":
		out := []any{}
		if get("movieId") == "2" && eventAllowed(q, "downloadFolderImported") {
			out = append(out, map[string]any{"eventType": "downloadFolderImported", "downloadId": strings.ToUpper(duneGrabHash), "quality": quality(1080), "data": map[string]string{"droppedPath": "/downloads/Movies/" + duneName + "/" + duneName + ".mkv", "size": "7516192768"}})
		}
		return out, true
	case path == "moviefile":
		id, _ := strconv.Atoi(get("movieId"))
		out := []any{}
		if f, ok := radarrFiles[id]; ok {
			out = append(out, map[string]any{"movieId": id, "size": f[0], "mediaInfo": map[string]any{"videoBitrate": f[1]}})
		}
		return out, true
	}
	return nil, false
}

var series = map[string]any{"id": 1, "title": "Game of Thrones", "originalTitle": "Game of Thrones", "year": 2011, "imdbId": "tt0944947", "tmdbId": 1399, "tvdbId": 121361, "runtime": 60, "alternateTitles": []any{}, "seasons": []any{map[string]any{"seasonNumber": 1, "statistics": map[string]int{"totalEpisodeCount": 10}}}}

func sonarr(path string, q url.Values) (any, bool) {
	get := q.Get
	switch path {
	case "system/status":
		return map[string]string{"appName": "Sonarr", "version": "4.0.10-demo"}, true
	case "history":
		records := []any{}
		if strings.EqualFold(get("downloadId"), episodeHash) && eventAllowed(q, "grabbed") {
			record := map[string]any{"downloadId": strings.ToUpper(episodeHash), "seriesId": 1, "eventType": "grabbed", "quality": quality(1080)}
			include(record, q, "includeSeries", "series", series)
			include(record, q, "includeEpisode", "episode", map[string]int{"seasonNumber": 1, "episodeNumber": 1})
			records = append(records, record)
		}
		return map[string]any{"page": 1, "pageSize": 100, "totalRecords": len(records), "records": records}, true
	case "series/1":
		return series, true
	case "parse":
		return map[string]any{"title": get("title")}, true
	case "series/lookup":
		out := []any{}
		if strings.EqualFold(strings.TrimSpace(get("term")), "Game of Thrones") {
			out = append(out, series)
		}
		return out, true
	case "history/series":
		return []any{}, true
	case "episodefile":
		out := []any{}
		if get("seriesId") == "1" {
			out = append(out, map[string]any{"seriesId": 1, "size": 2147483648, "mediaInfo": map[string]any{"videoBitrate": 9000000}})
		}
		return out, true
	}
	return nil, false
}
