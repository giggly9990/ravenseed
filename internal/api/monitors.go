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

func (a *API) trackerHosts(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), a.Service.Config.SearchTimeout)
	defer cancel()
	v, err := a.Service.TrackerHosts(ctx)
	if err != nil {
		serviceError(w, err, "qui_failed", "")
		return
	}
	write(w, 200, v)
}

func (a *API) monitors(w http.ResponseWriter, _ *http.Request) {
	v, err := a.Service.Store.Monitors()
	if err != nil {
		fail(w, 500, "database_error", "Could not load monitors")
		return
	}
	write(w, 200, v)
}

func (a *API) createMonitor(w http.ResponseWriter, r *http.Request) {
	var input models.CreateMonitorRequest
	if !decode(w, r, &input) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.Service.Config.SearchTimeout)
	defer cancel()
	v, err := a.Service.CreateMonitor(ctx, input.SourceIndexerID, input.TargetIndexerID, input.SourceTrackerHost, input.IntervalHours, input.InstanceIDs)
	if err != nil {
		serviceError(w, err, "monitor_failed", "")
		return
	}
	write(w, 201, v)
}

func (a *API) setMonitorState(w http.ResponseWriter, r *http.Request) {
	var input models.MonitorStateRequest
	if !decode(w, r, &input) {
		return
	}
	id := chi.URLParam(r, "monitorId")
	if input.InstanceIDs != nil {
		ctx, cancel := context.WithTimeout(r.Context(), a.Service.Config.SearchTimeout)
		defer cancel()
		v, err := a.Service.SetMonitorInstances(ctx, id, *input.InstanceIDs, input.Enabled)
		if err != nil {
			serviceError(w, err, "monitor_failed", "Monitor not found")
			return
		}
		write(w, 200, v)
		return
	}
	if input.Enabled == nil {
		fail(w, 400, "invalid_state", "send enabled as true or false, instanceIds, or both")
		return
	}
	v, err := a.Service.Store.SetMonitorEnabled(id, *input.Enabled)
	if err != nil {
		storeError(w, err, "Monitor not found", "Could not update monitor")
		return
	}
	write(w, 200, v)
}

func (a *API) deleteMonitor(w http.ResponseWriter, r *http.Request) {
	ok, err := a.Service.Store.DeleteMonitor(chi.URLParam(r, "monitorId"))
	if err != nil {
		fail(w, 500, "database_error", "Could not delete monitor")
		return
	}
	if !ok {
		fail(w, 404, "not_found", "Monitor not found")
		return
	}
	write(w, 200, map[string]bool{"deleted": true})
}

func (a *API) scanMonitor(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), a.Service.Config.SearchTimeout)
	defer cancel()
	if err := a.Service.ScanMonitor(ctx, chi.URLParam(r, "monitorId")); err != nil {
		if errors.Is(err, search.ErrMonitorPaused) {
			fail(w, 409, "monitor_paused", err.Error())
			return
		}
		serviceError(w, err, "scan_failed", "Monitor not found")
		return
	}
	write(w, 200, map[string]bool{"scanned": true})
}

func (a *API) monitorCandidates(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "monitorId")
	if _, err := a.Service.Store.Monitor(id); err != nil {
		storeError(w, err, "Monitor not found", "Could not load monitor")
		return
	}
	v, err := a.Service.Store.MonitorCandidates(id)
	if err != nil {
		fail(w, 500, "database_error", "Could not load candidates")
		return
	}
	q := r.URL.Query()
	// Name search spans every page; separators, case and accents are ignored.
	needle := releases.TitleKey(q.Get("q"))
	order := q.Get("sort")
	if order != "" && order != "checked_desc" && order != "checked_asc" {
		fail(w, 400, "invalid_sort", "Sort by checked_desc or checked_asc")
		return
	}
	if len(needle) > 200 {
		fail(w, 400, "invalid_query", "Search text is too long")
		return
	}
	v = slices.DeleteFunc(v, func(c models.MonitorCandidate) bool {
		if needle != "" && !strings.Contains(releases.TitleKey(c.Name), needle) && !strings.Contains(releases.TitleKey(c.Parsed.Group), needle) {
			return true
		}
		if value := q.Get("classification"); value != "" && value != "all" {
			switch value {
			case "errors", "gone":
				if c.Status != strings.TrimSuffix(value, "s") {
					return true
				}
			case "queued":
				if c.Status != "queued" && c.Status != "checking" {
					return true
				}
			default:
				if c.Policy == nil || string(c.Policy.Classification) != value {
					return true
				}
			}
		}
		if value := q.Get("releaseClass"); value != "" && c.Parsed.ReleaseType != value {
			return true
		}
		if value := q.Get("resolution"); value != "" && c.Parsed.Resolution != value {
			return true
		}
		if value := q.Get("source"); value != "" && c.Parsed.Source != value {
			return true
		}
		if value := q.Get("hdr"); value != "" && !slices.Contains(c.Parsed.HDR, value) {
			return true
		}
		if value := q.Get("group"); value != "" && !strings.EqualFold(c.Parsed.Group, value) {
			return true
		}
		if value := q.Get("policy"); value != "" && (c.Policy == nil || c.Policy.PolicySnapshotID != value) {
			return true
		}
		return false
	})
	if order != "" {
		// Never-checked torrents sort last either way.
		slices.SortStableFunc(v, func(a, b models.MonitorCandidate) int {
			switch {
			case a.LastCheckedAt == nil && b.LastCheckedAt == nil:
				return 0
			case a.LastCheckedAt == nil:
				return 1
			case b.LastCheckedAt == nil:
				return -1
			}
			if order == "checked_asc" {
				return a.LastCheckedAt.Compare(*b.LastCheckedAt)
			}
			return b.LastCheckedAt.Compare(*a.LastCheckedAt)
		})
	}
	page(w, r, v)
}

// monitorCandidateCounts returns, for every monitor, the total the
// candidates list reports for each classification filter.
func (a *API) monitorCandidateCounts(w http.ResponseWriter, r *http.Request) {
	v, err := a.Service.Store.MonitorCandidateCounts()
	if err != nil {
		fail(w, 500, "database_error", "Could not count candidates")
		return
	}
	write(w, 200, v)
}

func (a *API) checkMonitorCandidate(w http.ResponseWriter, r *http.Request) {
	var input struct{}
	if !decode(w, r, &input) {
		return
	}
	err := a.Service.QueueMonitorCheck(chi.URLParam(r, "candidateId"))
	if errors.Is(err, search.ErrMonitorPaused) || errors.Is(err, search.ErrCheckRunning) {
		fail(w, 409, "check_failed", err.Error())
		return
	}
	if err != nil {
		serviceError(w, err, "check_failed", "Candidate not found")
		return
	}
	write(w, 202, map[string]bool{"queued": true})
}

func (a *API) uploadMonitorCandidate(w http.ResponseWriter, r *http.Request) {
	var input models.MonitorUploadRequest
	if !decode(w, r, &input) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	v, err := a.Service.QueueExistingUpload(ctx, chi.URLParam(r, "candidateId"), input)
	if err != nil {
		serviceError(w, err, "upload_failed", "Candidate not found")
		return
	}
	write(w, 202, v)
}
