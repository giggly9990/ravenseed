package main

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
)

func metadataRoutes() {
	http.HandleFunc("/tmdb/3/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer demo-metadata-key" {
			w.WriteHeader(401)
			return
		}
		q := r.URL.Query()
		// TMDb rejects a find request that does not name the ID's source.
		if strings.HasPrefix(r.URL.Path, "/tmdb/3/find/") && q.Get("external_source") != "imdb_id" {
			writeJSON(w, 400, map[string]any{"success": false, "status_message": "external_source is required"})
			return
		}
		// external_ids is embedded only when append_to_response asks for it.
		externalIDs := slices.Contains(strings.Split(q.Get("append_to_response"), ","), "external_ids")
		detail := func(out map[string]any, ids map[string]any) map[string]any {
			if externalIDs {
				out["external_ids"] = ids
			}
			return out
		}
		var out any
		switch r.URL.Path {
		case "/tmdb/3/configuration":
			out = map[string]any{"images": map[string]string{"secure_base_url": "https://image.tmdb.org/t/p/"}}
		case "/tmdb/3/find/tt0133093":
			out = map[string]any{"movie_results": []any{map[string]int{"id": 603}}, "tv_results": []any{}}
		case "/tmdb/3/find/tt1160419":
			out = map[string]any{"movie_results": []any{map[string]int{"id": 438631}}, "tv_results": []any{}}
		case "/tmdb/3/find/tt2543164":
			out = map[string]any{"movie_results": []any{map[string]int{"id": 329865}}, "tv_results": []any{}}
		case "/tmdb/3/find/tt0944947":
			out = map[string]any{"movie_results": []any{}, "tv_results": []any{map[string]int{"id": 1399}}}
		case "/tmdb/3/movie/603":
			out = detail(map[string]any{"id": 603, "title": "The Matrix", "original_title": "The Matrix", "release_date": "1999-03-31", "runtime": 136, "imdb_id": "tt0133093", "overview": "Demo metadata for the mock discovery workflow."}, map[string]any{"imdb_id": "tt0133093"})
		case "/tmdb/3/movie/438631":
			out = detail(map[string]any{"id": 438631, "title": "Dune", "original_title": "Dune", "release_date": "2021-09-15", "runtime": 155, "imdb_id": "tt1160419", "overview": "Demo metadata for the tracker monitor upgrade example."}, map[string]any{"imdb_id": "tt1160419"})
		case "/tmdb/3/movie/329865":
			out = detail(map[string]any{"id": 329865, "title": "Arrival", "original_title": "Arrival", "release_date": "2016-11-10", "runtime": 116, "imdb_id": "tt2543164", "overview": "Demo metadata for the tracker monitor cross-seed example."}, map[string]any{"imdb_id": "tt2543164"})
		case "/tmdb/3/tv/1399":
			out = detail(map[string]any{"id": 1399, "name": "Game of Thrones", "original_name": "Game of Thrones", "first_air_date": "2011-04-17", "episode_run_time": []int{60}, "overview": "Demo series metadata.", "seasons": []any{map[string]any{"season_number": 1, "name": "Season 1", "episode_count": 10}}}, map[string]any{"imdb_id": "tt0944947", "tvdb_id": 121361})
		case "/tmdb/3/tv/1399/season/1":
			episodes := []any{}
			for n := 1; n <= 10; n++ {
				episodes = append(episodes, map[string]any{"season_number": 1, "episode_number": n, "name": "Episode " + strconv.Itoa(n), "runtime": 60})
			}
			out = map[string]any{"season_number": 1, "name": "Season 1", "air_date": "2011-04-17", "overview": "Demo season metadata.", "episodes": episodes}
		default:
			w.WriteHeader(404)
			return
		}
		writeJSON(w, 200, out)
	})
}
