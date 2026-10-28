package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"ravenseed/internal/models"
	"ravenseed/internal/policy"
	"ravenseed/internal/search"
	"slices"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

func policyIndexerID(w http.ResponseWriter, r *http.Request) (int, bool) {
	id, err := strconv.Atoi(chi.URLParam(r, "indexerId"))
	if err != nil || id <= 0 {
		fail(w, 400, "invalid_indexer", "Choose a valid indexer")
		return 0, false
	}
	return id, true
}

func (a *API) indexerPolicies(w http.ResponseWriter, _ *http.Request) {
	v, err := a.Service.Store.IndexerPolicies()
	if err != nil {
		fail(w, 500, "database_error", "Could not load indexer policies")
		return
	}
	write(w, 200, v)
}

func (a *API) policyPresets(w http.ResponseWriter, _ *http.Request) {
	write(w, 200, policy.Presets())
}

func (a *API) indexerPolicy(w http.ResponseWriter, r *http.Request) {
	id, ok := policyIndexerID(w, r)
	if !ok {
		return
	}
	v, err := a.Service.Store.IndexerPolicy(id)
	if errors.Is(err, sql.ErrNoRows) {
		v = policy.Default(id)
	} else if err != nil {
		fail(w, 500, "database_error", "Could not load indexer policy")
		return
	}
	write(w, 200, v)
}

func (a *API) saveIndexerPolicy(w http.ResponseWriter, r *http.Request) {
	id, ok := policyIndexerID(w, r)
	if !ok {
		return
	}
	var v models.IndexerPolicy
	if !decode(w, r, &v) {
		return
	}
	v.IndexerID = id
	if err := policy.Validate(v); err != nil {
		fail(w, 400, "invalid_policy", err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	indexers, err := a.Service.Indexers(ctx, false)
	if err != nil {
		fail(w, 502, "qui_failed", "Could not verify the target indexer")
		return
	}
	if !slices.ContainsFunc(indexers, func(i models.Indexer) bool { return i.ID == id && i.Enabled }) {
		fail(w, 400, "invalid_indexer", "Policy target must be an enabled qui indexer")
		return
	}
	v, err = a.Service.Store.SaveIndexerPolicy(v)
	if err != nil {
		fail(w, 500, "database_error", "Could not save indexer policy")
		return
	}
	write(w, 200, v)
}

func (a *API) deleteIndexerPolicy(w http.ResponseWriter, r *http.Request) {
	id, ok := policyIndexerID(w, r)
	if !ok {
		return
	}
	deleted, err := a.Service.Store.DeleteIndexerPolicy(id)
	if err != nil {
		fail(w, 500, "database_error", "Could not delete indexer policy")
		return
	}
	if !deleted {
		fail(w, 404, "not_found", "Policy not found")
		return
	}
	write(w, 200, map[string]bool{"deleted": true})
}

func (a *API) recheckIndexerPolicy(w http.ResponseWriter, r *http.Request) {
	id, ok := policyIndexerID(w, r)
	if !ok {
		return
	}
	if _, err := a.Service.Store.IndexerPolicy(id); err != nil {
		storeError(w, err, "Policy not found", "Could not load policy")
		return
	}
	count, err := a.Service.Store.RequeuePolicyCandidates(id, time.Now())
	if err != nil {
		fail(w, 500, "database_error", "Could not queue monitor rechecks")
		return
	}
	write(w, 202, map[string]int64{"queued": count})
}

func (a *API) evaluateIndexerPolicy(w http.ResponseWriter, r *http.Request) {
	id, ok := policyIndexerID(w, r)
	if !ok {
		return
	}
	var input struct {
		SearchID          string   `json:"searchId"`
		CandidateResultID string   `json:"candidateResultId"`
		TargetResultIDs   []string `json:"targetResultIds"`
	}
	if !decode(w, r, &input) {
		return
	}
	if input.SearchID == "" || input.CandidateResultID == "" {
		fail(w, 400, "invalid_input", "Choose a saved search and candidate result")
		return
	}
	v, err := a.Service.Store.IndexerPolicy(id)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, 400, "policy_disabled", "Enable and save an indexer policy first")
		return
	}
	if err != nil {
		fail(w, 500, "database_error", "Could not load policy")
		return
	}
	if !v.Enabled {
		fail(w, 400, "policy_disabled", "Enable and save an indexer policy first")
		return
	}
	job, err := a.Service.Store.Job(input.SearchID)
	if err != nil {
		storeError(w, err, "Search for this target was not found", "Could not load search")
		return
	}
	if job.Request.TargetIndexerID != id {
		fail(w, 404, "not_found", "Search for this target was not found")
		return
	}
	results, err := a.Service.Store.Results(input.SearchID)
	if err != nil {
		fail(w, 500, "database_error", "Could not load search results")
		return
	}
	var candidate models.Result
	found := false
	target := []models.Result{}
	for _, result := range results {
		if result.ID == input.CandidateResultID && result.IndexerID != id {
			candidate, found = result, true
		}
		if result.IndexerID == id && (len(input.TargetResultIDs) == 0 || slices.Contains(input.TargetResultIDs, result.ID)) {
			target = append(target, result)
		}
	}
	if len(input.TargetResultIDs) > 0 {
		unique := map[string]bool{}
		for _, result := range target {
			unique[result.ID] = true
		}
		for _, requested := range input.TargetResultIDs {
			if !unique[requested] {
				fail(w, 400, "invalid_target_result", "Every selected target result must belong to this saved search and indexer")
				return
			}
		}
	}
	if !found {
		fail(w, 404, "not_found", "Candidate result was not found")
		return
	}
	complete := slices.ContainsFunc(job.Indexers, func(i models.IndexerProgress) bool { return i.IndexerID == id && i.Status == "completed" })
	write(w, 200, policy.EvaluateWith(v, candidate, target, complete, search.PolicyContext(nil, job.Media)))
}
