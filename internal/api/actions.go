package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"github.com/go-chi/chi/v5"
	"net/http"
	"ravenseed/internal/models"
	"ravenseed/internal/qui"
	"ravenseed/internal/search"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
)

func (a *API) presets(w http.ResponseWriter, r *http.Request) {
	items, err := a.Service.Store.Presets()
	if err != nil {
		fail(w, 500, "database_error", "Could not load presets")
		return
	}
	write(w, 200, items)
}

func (a *API) savePreset(w http.ResponseWriter, r *http.Request) {
	var p models.IndexerPreset
	if !decode(w, r, &p) {
		return
	}
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" || len(p.Name) > 80 || strings.ContainsFunc(p.Name, unicode.IsControl) || len(p.IndexerIDs) == 0 || len(p.IndexerIDs) > 1000 || p.TargetIndexerID < 0 {
		fail(w, 400, "invalid_preset", "Use a name of 1 to 80 characters and select at least one indexer")
		return
	}
	slices.Sort(p.IndexerIDs)
	p.IndexerIDs = slices.Compact(p.IndexerIDs)
	if p.IndexerIDs[0] <= 0 || (p.TargetIndexerID != 0 && !slices.Contains(p.IndexerIDs, p.TargetIndexerID)) {
		fail(w, 400, "invalid_preset", "Indexer IDs must be positive and the target must be selected")
		return
	}
	items, err := a.Service.Store.Presets()
	if err != nil {
		fail(w, 500, "database_error", "Could not load presets")
		return
	}
	if p.ID != "" && !slices.ContainsFunc(items, func(item models.IndexerPreset) bool { return item.ID == p.ID }) {
		fail(w, 404, "not_found", "Preset not found")
		return
	}
	if p.ID == "" {
		if len(items) >= 100 {
			fail(w, 400, "preset_limit", "Up to 100 presets are supported")
			return
		}
		var id [12]byte
		if _, err := rand.Read(id[:]); err != nil {
			fail(w, 500, "preset_error", "Could not create preset")
			return
		}
		p.ID = hex.EncodeToString(id[:])
	}
	if err := a.Service.Store.SavePreset(p); err != nil {
		fail(w, 400, "preset_save_failed", "Could not save preset; use a unique name")
		return
	}
	write(w, 200, p)
}

func (a *API) deletePreset(w http.ResponseWriter, r *http.Request) {
	deleted, err := a.Service.Store.DeletePreset(chi.URLParam(r, "presetId"))
	if err != nil {
		fail(w, 500, "database_error", "Could not delete preset")
		return
	}
	if !deleted {
		fail(w, 404, "not_found", "Preset not found")
		return
	}
	write(w, 200, map[string]bool{"deleted": true})
}

func (a *API) instances(w http.ResponseWriter, r *http.Request) {
	c, err := a.Service.Connection(models.Settings{})
	if err != nil {
		fail(w, 400, "not_configured", err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	items, err := c.Instances(ctx)
	if err != nil {
		fail(w, 502, "qui_error", err.Error())
		return
	}
	write(w, 200, items)
}

func (a *API) categories(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "instanceId"))
	if err != nil || id <= 0 {
		fail(w, 400, "invalid_instance", "Choose a valid qui instance")
		return
	}
	c, err := a.Service.Connection(models.Settings{})
	if err != nil {
		fail(w, 400, "not_configured", err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	items, err := c.Categories(ctx, id)
	if err != nil {
		fail(w, 502, "qui_error", err.Error())
		return
	}
	write(w, 200, items)
}

// torrentDestination validates where a torrent is added: the instance (the
// default when omitted) must be connected and active, the category enabled,
// and the options well formed. It answers the request on failure.
func (a *API) torrentDestination(ctx context.Context, w http.ResponseWriter, input *models.AddTorrentRequest) (*qui.Client, bool) {
	preferences, err := a.Service.Store.TorrentSettings()
	if err != nil {
		fail(w, 500, "database_error", "Could not read torrent settings")
		return nil, false
	}
	if input.InstanceID == 0 {
		input.InstanceID = preferences.DefaultInstanceID
	}
	if !preferences.AllowsCategory(input.InstanceID, input.Category) {
		fail(w, 400, "category_disabled", "This category is disabled for the selected instance")
		return nil, false
	}
	if input.InstanceID <= 0 || len(input.Category) > 256 || len(input.SavePath) > 4096 || len(input.Tags) > 1024 || strings.ContainsFunc(input.Category+input.SavePath+input.Tags, unicode.IsControl) {
		fail(w, 400, "invalid_options", "Choose an instance and valid torrent options")
		return nil, false
	}
	c, err := a.Service.Connection(models.Settings{})
	if err != nil {
		fail(w, 400, "not_configured", err.Error())
		return nil, false
	}
	instances, err := c.Instances(ctx)
	if err != nil {
		fail(w, 502, "qui_error", err.Error())
		return nil, false
	}
	if !slices.ContainsFunc(instances, func(i models.QuiInstance) bool { return i.ID == input.InstanceID && i.IsActive && i.Connected }) {
		fail(w, 400, "instance_unavailable", "Choose a connected, active qui instance")
		return nil, false
	}
	return c, true
}

func (a *API) addTorrent(w http.ResponseWriter, r *http.Request) {
	j, ok := a.load(w, r)
	if !ok {
		return
	}
	var input models.AddTorrentRequest
	if !decode(w, r, &input) {
		return
	}
	results, err := a.Service.Store.Results(j.ID)
	if err != nil {
		fail(w, 500, "database_error", "Could not load saved result")
		return
	}
	idx := slices.IndexFunc(results, func(result models.Result) bool { return result.ID == chi.URLParam(r, "resultId") })
	if idx < 0 {
		fail(w, 404, "not_found", "Saved result not found")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 75*time.Second)
	defer cancel()
	c, ok := a.torrentDestination(ctx, w, &input)
	if !ok {
		return
	}
	var out models.AddTorrentResponse
	if input.UploadAfterDownload {
		out, err = a.Service.QueueUpload(ctx, j, results[idx], input)
	} else if out, err = c.AddTorrent(ctx, results[idx], input); err != nil && !errors.Is(err, qui.ErrNoDownloadLink) {
		err = search.UpstreamError{Err: err}
	}
	if err != nil {
		serviceError(w, err, "add_failed", "")
		return
	}
	write(w, 200, out)
}
