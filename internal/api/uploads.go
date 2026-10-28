package api

import (
	"context"
	"github.com/go-chi/chi/v5"
	"net/http"
	"ravenseed/internal/models"
	"slices"
	"strings"
	"time"
)

func (a *API) uploadSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		v, err := a.Service.UploadSettings()
		if err != nil {
			fail(w, 500, "database_error", "Could not read upload settings")
			return
		}
		write(w, 200, v)
		return
	}
	var v models.UploadSettings
	if !decode(w, r, &v) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err := a.Service.ConfigureUploads(ctx, v, r.Method == "PUT"); err != nil {
		serviceError(w, err, "upload_settings_failed", "")
		return
	}
	if r.Method == "POST" {
		write(w, 200, map[string]bool{"connected": true})
		return
	}
	v, err := a.Service.UploadSettings()
	if err != nil {
		fail(w, 500, "database_error", "Upload settings were saved but could not be read back")
		return
	}
	write(w, 200, v)
}

// UploadJobsPage is a page of upload jobs with the number of jobs in each
// status, counted over the whole queue whatever the status filter.
type UploadJobsPage struct {
	Page[models.UploadJob]
	StatusCounts map[string]int `json:"statusCounts"`
}

func (a *API) uploads(w http.ResponseWriter, r *http.Request) {
	limit, offset, ok := pagination(w, r)
	if !ok {
		return
	}
	var statuses []string
	for _, v := range strings.Split(strings.Join(r.URL.Query()["status"], ","), ",") {
		if v = strings.TrimSpace(v); v == "" {
			continue
		}
		if !slices.Contains(models.UploadStatuses, v) {
			fail(w, 400, "invalid_filter", "Unknown upload status; use "+strings.Join(models.UploadStatuses, ", "))
			return
		}
		statuses = append(statuses, v)
	}
	items, total, counts, err := a.Service.Store.UploadQueue(limit, offset, statuses)
	if err != nil {
		fail(w, 500, "database_error", "Could not read upload queue")
		return
	}
	write(w, 200, UploadJobsPage{Page: Page[models.UploadJob]{Items: items, Total: total, Limit: limit, Offset: offset}, StatusCounts: counts})
}

func (a *API) cancelUpload(w http.ResponseWriter, r *http.Request) {
	var input struct{}
	if !decode(w, r, &input) {
		return
	}
	if err := a.Service.CancelUpload(chi.URLParam(r, "uploadId")); err != nil {
		serviceError(w, err, "cancel_failed", "Upload job not found")
		return
	}
	write(w, 200, map[string]bool{"cancelled": true})
}

func (a *API) retryUpload(w http.ResponseWriter, r *http.Request) {
	var input models.RetryUploadRequest
	if !decode(w, r, &input) {
		return
	}
	j, err := a.Service.RetryUpload(chi.URLParam(r, "uploadId"), input)
	if err != nil {
		serviceError(w, err, "retry_failed", "Upload job not found")
		return
	}
	write(w, 200, j)
}
