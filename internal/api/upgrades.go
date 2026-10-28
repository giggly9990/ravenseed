package api

import (
	"context"
	"errors"
	"net/http"
	"ravenseed/internal/models"
	"ravenseed/internal/releases"
	"ravenseed/internal/search"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// upgrades lists monitor torrents whose slot on the target holds a release
// that may replace them, across every monitor.
func (a *API) upgrades(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	level, monitorID := q.Get("level"), q.Get("monitorId")
	if level != "" && level != search.UpgradeObjective && level != search.UpgradePossible {
		fail(w, 400, "invalid_level", "level must be upgrade or possible")
		return
	}
	dismissed := q.Get("dismissed") == "true"
	if len(q.Get("q")) > 200 {
		fail(w, 400, "invalid_query", "Search text is too long")
		return
	}
	// Name search covers the torrent, the offered release and either group.
	needle := releases.TitleKey(q.Get("q"))
	items, err := a.Service.Store.UpgradeCandidates(monitorID)
	if err != nil {
		storeError(w, err, "", "Could not load upgrades")
		return
	}
	items = slices.DeleteFunc(items, func(c models.MonitorCandidate) bool {
		return c.Upgrade == "" || c.UpgradeOffer == nil ||
			(c.UpgradeDismissed != "") != dismissed ||
			(level != "" && c.Upgrade != level) ||
			(needle != "" && !slices.ContainsFunc([]string{c.Name, c.UpgradeOffer.Title, c.Parsed.Group, c.UpgradeOffer.Group}, func(text string) bool {
				return text != "" && strings.Contains(releases.TitleKey(text), needle)
			}))
	})
	// Rule-backed upgrades first, then the most recently checked; never
	// checked torrents last.
	slices.SortStableFunc(items, func(x, y models.MonitorCandidate) int {
		if x.Upgrade != y.Upgrade {
			if x.Upgrade == search.UpgradeObjective {
				return -1
			}
			return 1
		}
		switch {
		case x.LastCheckedAt == nil && y.LastCheckedAt == nil:
			return 0
		case x.LastCheckedAt == nil:
			return 1
		case y.LastCheckedAt == nil:
			return -1
		default:
			return y.LastCheckedAt.Compare(*x.LastCheckedAt)
		}
	})
	page(w, r, items)
}

func (a *API) dismissUpgrade(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Dismissed bool `json:"dismissed"`
	}
	if !decode(w, r, &input) {
		return
	}
	v, err := a.Service.SetUpgradeDismissed(chi.URLParam(r, "candidateId"), input.Dismissed)
	if errors.Is(err, search.ErrUpgradeChanged) {
		fail(w, 409, "upgrade_changed", err.Error())
		return
	}
	if err != nil {
		serviceError(w, err, "upgrade_failed", "Candidate not found")
		return
	}
	write(w, 200, v)
}

func (a *API) addUpgrade(w http.ResponseWriter, r *http.Request) {
	var input models.AddTorrentRequest
	if !decode(w, r, &input) {
		return
	}
	if input.UploadAfterDownload {
		fail(w, 400, "invalid_options", "Upgrades are added without an upload")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 75*time.Second)
	defer cancel()
	if _, ok := a.torrentDestination(ctx, w, &input); !ok {
		return
	}
	out, err := a.Service.AddUpgrade(ctx, chi.URLParam(r, "candidateId"), input)
	if err != nil {
		serviceError(w, err, "add_failed", "Candidate not found")
		return
	}
	write(w, 200, out)
}
