package api

import (
	"context"
	"net/http"
	"ravenseed/internal/models"
	"time"
)

func (a *API) resolveMedia(w http.ResponseWriter, r *http.Request) {
	var input models.ResolveRequest
	if !decode(w, r, &input) {
		return
	}
	media, err := a.Service.Resolve(r.Context(), input)
	if err != nil {
		serviceError(w, err, "invalid_media", "")
		return
	}
	write(w, 200, media)
}

func (a *API) metadataSettings(w http.ResponseWriter, r *http.Request) {
	var input models.MetadataSettings
	if !decode(w, r, &input) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := a.Service.SaveMetadataSettings(ctx, input); err != nil {
		serviceError(w, err, "metadata_settings_failed", "")
		return
	}
	write(w, 200, a.Service.Settings())
}

func (a *API) testMetadata(w http.ResponseWriter, r *http.Request) {
	var input models.MetadataSettings
	if !decode(w, r, &input) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := a.Service.TestMetadata(ctx, input); err != nil {
		serviceError(w, err, "metadata_test_failed", "")
		return
	}
	write(w, 200, map[string]bool{"connected": true})
}
