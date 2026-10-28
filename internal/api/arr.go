package api

import (
	"context"
	"net/http"
	"ravenseed/internal/models"
	"time"
)

func (a *API) arrSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		v, err := a.Service.ArrSettings()
		if err != nil {
			fail(w, 500, "database_error", "Could not read Sonarr and Radarr settings")
			return
		}
		write(w, 200, v)
		return
	}
	var v models.ArrSettings
	if !decode(w, r, &v) {
		return
	}
	saved, err := a.Service.SaveArrSettings(v)
	if err != nil {
		serviceError(w, err, "arr_settings_failed", "")
		return
	}
	write(w, 200, saved)
}

func (a *API) testArr(w http.ResponseWriter, r *http.Request) {
	var v models.ArrInstance
	if !decode(w, r, &v) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	if err := a.Service.TestArrInstance(ctx, v); err != nil {
		serviceError(w, err, "arr_test_failed", "")
		return
	}
	write(w, 200, map[string]bool{"connected": true})
}
