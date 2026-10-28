package api

import (
	"net/http"
	"ravenseed/internal/models"
)

func (a *API) searchOverrides(w http.ResponseWriter, r *http.Request) {
	var v map[int]models.IndexerSearchOverride
	if !decode(w, r, &v) {
		return
	}
	if err := a.Service.SaveSearchOverrides(v); err != nil {
		serviceError(w, err, "invalid_search_overrides", "")
		return
	}
	write(w, 200, a.Service.Settings())
}
