package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

func uploadRoutes() {
	http.HandleFunc("/api/instances/1/torrents/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "demo-key" {
			w.WriteHeader(401)
			return
		}
		if !strings.HasSuffix(r.URL.Path, "/files") {
			http.NotFound(w, r)
			return
		}
		hash := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/instances/1/torrents/"), "/")[0]
		files := []any{}
		switch t := torrentByHash(hash); {
		case t != nil:
			for _, f := range t.files {
				files = append(files, map[string]any{"name": f.name, "size": f.size, "progress": 1})
			}
		case strings.EqualFold(hash, downloadedHash) && hasDownloads():
			files = append(files, map[string]any{"name": "movie.mkv", "size": 1000, "progress": 1})
		default:
			http.NotFound(w, r)
			return
		}
		writeJSON(w, 200, files)
	})
	http.HandleFunc("/ua/api/browse_roots", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer demo-upload-key" {
			w.WriteHeader(401)
			return
		}
		writeJSON(w, 200, map[string]any{"success": true, "items": []any{}})
	})
	http.HandleFunc("/ua/api/execute", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer demo-upload-key" {
			w.WriteHeader(401)
			return
		}
		var v map[string]string
		if r.Method != "POST" || json.NewDecoder(r.Body).Decode(&v) != nil || v["path"] != "/data/Demo/movie.mkv" || !strings.Contains(v["args"], "--trackers DEMO") || !strings.HasPrefix(v["session_id"], "upload_") {
			w.WriteHeader(400)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"gui\",\"data\":{\"kind\":\"stage\",\"key\":\"screens\",\"status\":\"start\"}}\n\n" +
			"data: {\"type\":\"progress\",\"data\":{\"group\":\"screenshots\"}}\n\n" +
			"data: {\"type\":\"gui\",\"data\":{\"kind\":\"tracker\",\"tracker\":\"DEMO\",\"status\":\"success\",\"link\":\"https://demo.example/torrents/1\",\"torrent_id\":\"1\"}}\n\n" +
			"data: {\"type\":\"exit\",\"code\":0}\n\n"))
	})
}
