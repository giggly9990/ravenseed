package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"ravenseed/internal/models"
	"ravenseed/internal/ranking"
	"ravenseed/internal/search"
)

type API struct {
	Service  *search.Service
	sessions *browserSessions
}
type ErrorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}
type Page[T any] struct {
	Items  []T `json:"items"`
	Total  int `json:"total"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

func New(s *search.Service, web string) http.Handler {
	a := &API{Service: s, sessions: newBrowserSessions()}
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(a.headers)
	r.Use(a.protect)
	r.Get("/login", a.login)
	r.Post("/login", a.login)
	r.Post("/logout", a.logout)
	r.Get("/health", a.health)
	r.Get("/api/v1/health", a.health)
	r.Get("/api/openapi.json", func(w http.ResponseWriter, r *http.Request) { write(w, 200, openAPISpec()) })
	r.Get("/api/docs", func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, filepath.Join(web, "docs.html")) })
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(rateLimit(s.Config.TrustedProxies...))
		r.Get("/ready", a.ready)
		r.Get("/session", func(w http.ResponseWriter, r *http.Request) {
			write(w, 200, map[string]bool{"enabled": s.Config.AuthRequired(), "authenticated": a.session(r) != nil})
		})
		r.Get("/settings", func(w http.ResponseWriter, r *http.Request) { write(w, 200, s.Settings()) })
		r.Put("/settings", a.settings)
		r.Get("/settings/torrents", a.torrentSettings)
		r.Put("/settings/torrents", a.torrentSettings)
		r.Put("/settings/matching", a.matchingSettings)
		r.Put("/settings/search-overrides", a.searchOverrides)
		r.Put("/settings/metadata", a.metadataSettings)
		r.Post("/settings/metadata/test", a.testMetadata)
		r.Get("/settings/uploads", a.uploadSettings)
		r.Put("/settings/uploads", a.uploadSettings)
		r.Post("/settings/uploads/test", a.uploadSettings)
		r.Get("/settings/arr", a.arrSettings)
		r.Put("/settings/arr", a.arrSettings)
		r.Post("/settings/arr/test", a.testArr)
		r.Get("/uploads", a.uploads)
		r.Get("/tracker-hosts", a.trackerHosts)
		r.Get("/monitors", a.monitors)
		r.Post("/monitors", a.createMonitor)
		r.Put("/monitors/{monitorId}", a.setMonitorState)
		r.Delete("/monitors/{monitorId}", a.deleteMonitor)
		r.Post("/monitors/{monitorId}/scan", a.scanMonitor)
		r.Get("/monitors/{monitorId}/candidates", a.monitorCandidates)
		r.Get("/monitor-candidate-counts", a.monitorCandidateCounts)
		r.Post("/monitor-candidates/{candidateId}/check", a.checkMonitorCandidate)
		r.Post("/monitor-candidates/{candidateId}/upload", a.uploadMonitorCandidate)
		r.Get("/upgrades", a.upgrades)
		r.Post("/monitor-candidates/{candidateId}/upgrade/dismiss", a.dismissUpgrade)
		r.Post("/monitor-candidates/{candidateId}/upgrade/add", a.addUpgrade)
		r.Post("/uploads/{uploadId}/cancel", a.cancelUpload)
		r.Post("/uploads/{uploadId}/retry", a.retryUpload)
		r.Post("/media/resolve", a.resolveMedia)
		r.Post("/settings/test", a.testConnection)
		r.Get("/indexers", a.indexers)
		r.Get("/indexer-policies", a.indexerPolicies)
		r.Get("/policy-presets", a.policyPresets)
		r.Get("/indexer-policies/{indexerId}", a.indexerPolicy)
		r.Put("/indexer-policies/{indexerId}", a.saveIndexerPolicy)
		r.Delete("/indexer-policies/{indexerId}", a.deleteIndexerPolicy)
		r.Post("/indexer-policies/{indexerId}/evaluate", a.evaluateIndexerPolicy)
		r.Post("/indexer-policies/{indexerId}/recheck", a.recheckIndexerPolicy)
		r.Get("/presets", a.presets)
		r.Put("/presets", a.savePreset)
		r.Delete("/presets/{presetId}", a.deletePreset)
		r.Get("/qui/instances", a.instances)
		r.Get("/qui/instances/{instanceId}/categories", a.categories)
		r.Post("/searches", a.start)
		r.Get("/searches", a.history)
		r.Get("/searches/{id}", a.job)
		r.Get("/searches/{id}/events", a.events)
		r.Get("/searches/{id}/results", a.results)
		r.Post("/searches/{id}/results/{resultId}/add", a.addTorrent)
		r.Get("/searches/{id}/families", a.families)
		r.Get("/searches/{id}/families/{familyId}", a.family)
		r.Get("/searches/{id}/candidates", a.families)
	})
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			fail(w, 404, "not_found", "Endpoint not found")
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			fail(w, 405, "method_not_allowed", "Method not allowed")
			return
		}
		clean := filepath.Clean("/" + r.URL.Path)
		path := filepath.Join(web, clean)
		if stat, err := os.Stat(path); err == nil && !stat.IsDir() {
			http.ServeFile(w, r, path)
			return
		}
		if filepath.Ext(r.URL.Path) != "" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(web, "index.html"))
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) { fail(w, 405, "method_not_allowed", "Method not allowed") })
	return r
}
func (a *API) headers(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https://image.tmdb.org; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		if !a.allowedHost(r) && r.URL.Path != "/health" && r.URL.Path != "/api/v1/health" {
			fail(w, 403, "host_rejected", "This host name is not allowed while authentication is disabled; add it to ALLOWED_HOSTS or enable authentication")
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" && r.Header.Get("Origin") != "" {
			u, err := url.Parse(r.Header.Get("Origin"))
			if err != nil || !strings.EqualFold(u.Host, a.requestHost(r)) {
				fail(w, 403, "origin_rejected", "Cross-origin writes are not allowed")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
func rateLimit(trusted ...netip.Prefix) func(http.Handler) http.Handler {
	type entry struct {
		start time.Time
		n     int
	}
	var mu sync.Mutex
	clients := map[string]entry{}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host := clientAddress(r, trusted)
			mu.Lock()
			now := time.Now()
			if len(clients) > 10000 {
				for k, v := range clients {
					if now.Sub(v.start) > time.Minute {
						delete(clients, k)
					}
				}
			}
			e := clients[host]
			if now.Sub(e.start) > time.Minute {
				e = entry{start: now}
			}
			e.n++
			clients[host] = e
			mu.Unlock()
			if e.n > 240 {
				w.Header().Set("Retry-After", "60")
				fail(w, 429, "rate_limited", "Too many requests; retry in one minute")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, code, message string) {
	var b ErrorBody
	b.Error.Code = code
	b.Error.Message = message
	write(w, status, b)
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mediaType != "application/json" {
		fail(w, 415, "invalid_content_type", "Use application/json")
		return false
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			fail(w, 413, "body_too_large", "The request body is too large")
			return false
		}
		fail(w, 400, "invalid_json", "Invalid JSON or unknown fields")
		return false
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		fail(w, 400, "invalid_json", "Expected one JSON object")
		return false
	}
	return true
}
func (a *API) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := a.Service.Store.DB.PingContext(ctx); err != nil {
		write(w, 503, map[string]string{"status": "error", "database": "unavailable"})
		return
	}
	write(w, 200, map[string]string{"status": "ok", "database": "ok"})
}

func (a *API) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := a.Service.Store.DB.PingContext(ctx); err != nil {
		write(w, 503, map[string]string{"status": "error", "database": "unavailable", "qui": "unknown"})
		return
	}
	quiStatus := "not_configured"
	settings := a.Service.Settings()
	if settings.QuiURL != "" && settings.HasAPIKey {
		quiStatus = "unavailable"
		if _, err := a.Service.Indexers(ctx, false); err == nil {
			quiStatus = "connected"
		}
	}
	if quiStatus == "unavailable" {
		write(w, 503, map[string]string{"status": "degraded", "database": "ok", "qui": quiStatus})
		return
	}
	write(w, 200, map[string]string{"status": "ok", "database": "ok", "qui": quiStatus})
}
func (a *API) settings(w http.ResponseWriter, r *http.Request) {
	var v models.Settings
	if !decode(w, r, &v) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := a.Service.SaveSettings(ctx, v); err != nil {
		serviceError(w, err, "settings_failed", "")
		return
	}
	write(w, 200, a.Service.Settings())
}

func (a *API) matchingSettings(w http.ResponseWriter, r *http.Request) {
	var rules models.MatchingRules
	if !decode(w, r, &rules) {
		return
	}
	if err := a.Service.SaveMatching(rules); err != nil {
		serviceError(w, err, "invalid_matching_rules", "")
		return
	}
	write(w, 200, a.Service.Settings())
}
func (a *API) testConnection(w http.ResponseWriter, r *http.Request) {
	var v models.Settings
	if !decode(w, r, &v) {
		return
	}
	c, err := a.Service.Connection(v)
	if err != nil {
		fail(w, 400, "invalid_connection", err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	idx, err := c.Indexers(ctx)
	if err != nil {
		fail(w, 502, "qui_unavailable", err.Error())
		return
	}
	write(w, 200, map[string]any{"connected": true, "indexers": idx})
}
func (a *API) indexers(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	idx, err := a.Service.Indexers(ctx, r.URL.Query().Get("refresh") == "true")
	if err != nil {
		fail(w, 502, "qui_unavailable", err.Error())
		return
	}
	write(w, 200, idx)
}
func (a *API) start(w http.ResponseWriter, r *http.Request) {
	var input struct {
		models.SearchRequest
		Target *int `json:"targetIndexerId"`
	}
	if !decode(w, r, &input) {
		return
	}
	req := input.SearchRequest
	if input.Target == nil {
		req.TargetIndexerID = a.Service.Settings().DefaultTargetIndexer
	} else {
		req.TargetIndexerID = *input.Target
	}
	j, err := a.Service.Start(r.Context(), req)
	if errors.Is(err, search.ErrBusy) {
		w.Header().Set("Retry-After", "10")
		fail(w, 429, "search_failed", err.Error())
		return
	}
	if err != nil {
		serviceError(w, err, "search_failed", "")
		return
	}
	status := 202
	if j.CacheHit {
		status = 200
	}
	write(w, status, j)
}
func pagination(w http.ResponseWriter, r *http.Request) (int, int, bool) {
	limit, offset := 50, 0
	var err error
	if v := r.URL.Query().Get("limit"); v != "" {
		limit, err = strconv.Atoi(v)
		if err != nil || limit < 1 || limit > 200 {
			fail(w, 400, "invalid_pagination", "limit must be from 1 to 200")
			return 0, 0, false
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		offset, err = strconv.Atoi(v)
		if err != nil || offset < 0 {
			fail(w, 400, "invalid_pagination", "offset must be nonnegative")
			return 0, 0, false
		}
	}
	return limit, offset, true
}
func page[T any](w http.ResponseWriter, r *http.Request, items []T) {
	limit, offset, ok := pagination(w, r)
	if !ok {
		return
	}
	start := min(offset, len(items))
	end := start + min(limit, len(items)-start)
	write(w, 200, Page[T]{items[start:end], len(items), limit, offset})
}
func (a *API) history(w http.ResponseWriter, r *http.Request) {
	limit, offset, ok := pagination(w, r)
	if !ok {
		return
	}
	items, total, err := a.Service.Store.History(limit, offset)
	if err != nil {
		fail(w, 500, "database_error", "Could not load history")
		return
	}
	write(w, 200, Page[models.Job]{items, total, limit, offset})
}
func (a *API) load(w http.ResponseWriter, r *http.Request) (models.Job, bool) {
	j, err := a.Service.Store.Job(chi.URLParam(r, "id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			fail(w, 404, "not_found", "Search not found")
		} else {
			fail(w, 500, "database_error", "Could not load search")
		}
		return j, false
	}
	return j, true
}
func (a *API) job(w http.ResponseWriter, r *http.Request) {
	if j, ok := a.load(w, r); ok {
		write(w, 200, j)
	}
}
func (a *API) results(w http.ResponseWriter, r *http.Request) {
	j, ok := a.load(w, r)
	if !ok {
		return
	}
	rs, err := a.Service.Store.Results(j.ID)
	if err != nil {
		fail(w, 500, "database_error", "Could not load results")
		return
	}
	page(w, r, rs)
}
func (a *API) families(w http.ResponseWriter, r *http.Request) {
	j, ok := a.load(w, r)
	if !ok {
		return
	}
	fs, err := a.Service.Store.Families(j.ID)
	if err != nil {
		fail(w, 500, "database_error", "Could not load families")
		return
	}
	q := r.URL.Query()
	by := q.Get("sort")
	order := q.Get("order")
	if order != "" && order != "asc" && order != "desc" {
		fail(w, 400, "invalid_sort", "order must be asc or desc")
		return
	}
	if !slices.Contains([]string{"", "coverage", "quality", "seeders", "size", "newest", "target"}, by) {
		fail(w, 400, "invalid_sort", "Unknown sort order")
		return
	}
	minimum := j.Request.Filters.MinimumCoverage
	if v := q.Get("minimumCoverage"); v != "" {
		minimum, err = strconv.Atoi(v)
		if err != nil || minimum < 0 || minimum > 1000 {
			fail(w, 400, "invalid_filter", "Invalid minimumCoverage")
			return
		}
	}
	for _, name := range []string{"targetMissing", "exactOnly"} {
		if v := q.Get(name); v != "" && v != "true" && v != "false" {
			fail(w, 400, "invalid_filter", name+" must be true or false")
			return
		}
	}
	out := []models.Family{}
	for _, f := range fs {
		p := f.Canonical
		if f.Coverage < minimum {
			continue
		}
		if !matches(q.Get("resolution"), p.Resolution) || !matches(q.Get("releaseType"), p.ReleaseType) || !matches(q.Get("source"), p.Source) || !matches(q.Get("group"), p.Group) || !matches(q.Get("codec"), p.VideoCodec) || !matches(q.Get("confidence"), f.Confidence) {
			continue
		}
		if hdr := q.Get("hdr"); hdr != "" {
			formats := p.HDR
			if len(formats) == 0 {
				formats = []string{"SDR"}
			}
			if !slices.ContainsFunc(formats, func(v string) bool { return matches(hdr, v) }) {
				continue
			}
		}
		if (q.Get("targetMissing") == "true" || strings.HasSuffix(r.URL.Path, "/candidates")) && !f.TargetStatus.Candidate {
			continue
		}
		if q.Get("exactOnly") == "true" && f.Confidence != "EXACT" {
			continue
		}
		out = append(out, f)
	}
	if strings.HasSuffix(r.URL.Path, "/candidates") && by == "" {
		by = "target"
	}
	ranking.Sort(out, by)
	if order == "asc" {
		slices.Reverse(out)
	}
	page(w, r, out)
}
func matches(query, value string) bool {
	if query == "" {
		return true
	}
	return slices.ContainsFunc(strings.Split(query, ","), func(v string) bool { return strings.EqualFold(strings.TrimSpace(v), value) })
}
func (a *API) family(w http.ResponseWriter, r *http.Request) {
	j, ok := a.load(w, r)
	if !ok {
		return
	}
	fs, err := a.Service.Store.Families(j.ID)
	if err != nil {
		fail(w, 500, "database_error", "Could not load family")
		return
	}
	for _, f := range fs {
		if f.ID == chi.URLParam(r, "familyId") {
			write(w, 200, f)
			return
		}
	}
	fail(w, 404, "not_found", "Family not found")
}
func (a *API) events(w http.ResponseWriter, r *http.Request) {
	j, ok := a.load(w, r)
	if !ok {
		return
	}
	after := int64(0)
	if v := r.Header.Get("Last-Event-ID"); v != "" {
		var err error
		after, err = strconv.ParseInt(v, 10, 64)
		if err != nil || after < 0 {
			fail(w, 400, "invalid_cursor", "Invalid Last-Event-ID")
			return
		}
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		fail(w, 500, "stream_unavailable", "Streaming unavailable")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(200)
	flusher.Flush()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		// Status first: a search that finished before this read has stored
		// all its events, so the drain below cannot miss the last ones.
		current, err := a.Service.Store.Job(j.ID)
		if err != nil {
			return
		}
		events, err := a.Service.Store.Events(j.ID, after)
		if err != nil {
			return
		}
		for _, e := range events {
			_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(10 * time.Second))
			if _, err = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", e.ID, e.Name, e.Data); err != nil {
				return
			}
			after = e.ID
			flusher.Flush()
		}
		if current.Status != "running" && len(events) < 100 {
			_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(10 * time.Second))
			b, _ := json.Marshal(current)
			_, _ = fmt.Fprintf(w, "event: search.snapshot\ndata: %s\n\n", b)
			flusher.Flush()
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		case <-heartbeat.C:
			_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(10 * time.Second))
			if _, err = io.WriteString(w, ": heartbeat\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
