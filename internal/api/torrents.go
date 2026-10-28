package api

import (
	"net/http"
	"ravenseed/internal/models"
	"slices"
	"strings"
	"unicode"
)

func (a *API) torrentSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		v, err := a.Service.Store.TorrentSettings()
		if err != nil {
			fail(w, 500, "database_error", "Could not read torrent settings")
			return
		}
		write(w, 200, v)
		return
	}
	var v models.TorrentSettings
	if !decode(w, r, &v) {
		return
	}
	if v.DefaultInstanceID < 0 || len(v.Categories) > 100 {
		fail(w, 400, "invalid_settings", "Choose a valid default instance and up to 100 category lists")
		return
	}
	seen := map[int]bool{}
	for i := range v.Categories {
		rule := &v.Categories[i]
		if rule.InstanceID <= 0 || seen[rule.InstanceID] || len(rule.Enabled) > 1000 {
			fail(w, 400, "invalid_settings", "Use one category list per instance with up to 1000 categories")
			return
		}
		seen[rule.InstanceID] = true
		for _, name := range rule.Enabled {
			if strings.TrimSpace(name) == "" || len(name) > 256 || strings.ContainsFunc(name, unicode.IsControl) {
				fail(w, 400, "invalid_settings", "Use valid category names")
				return
			}
		}
		slices.Sort(rule.Enabled)
		rule.Enabled = slices.Compact(rule.Enabled)
		if rule.Enabled == nil {
			rule.Enabled = []string{}
		}
	}
	if v.Categories == nil {
		v.Categories = []models.InstanceCategories{}
	}
	if err := a.Service.Store.SaveTorrentSettings(v); err != nil {
		fail(w, 500, "database_error", "Could not save torrent settings")
		return
	}
	write(w, 200, v)
}
