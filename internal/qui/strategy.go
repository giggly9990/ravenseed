package qui

import (
	"errors"
	"fmt"
	"ravenseed/internal/models"
	"slices"
	"strconv"
	"strings"
)

func supports(idx models.Indexer, cap string) bool {
	return slices.ContainsFunc(idx.Capabilities, func(value string) bool { return strings.EqualFold(value, cap) })
}

func Strategy(m models.Media, idx models.Indexer) (models.SearchStrategy, error) {
	prefix := "movie-search-"
	methods := []string{"imdb", "tmdb"}
	if m.Type == "tv" {
		prefix = "tv-search-"
		methods = []string{"tvdb", "imdb", "tmdb"}
	}
	override := "auto"
	if idx.SearchOverride != nil {
		override = idx.SearchOverride.Movie
		if m.Type == "tv" {
			override = idx.SearchOverride.TV
		}
	}
	if override != "" && override != "auto" {
		switch override {
		case "imdb", "tmdb", "tvdb", "title":
		default:
			return models.SearchStrategy{Method: "unsupported"}, errors.New("invalid indexer search override")
		}
		if override == "tvdb" && m.Type != "tv" {
			return models.SearchStrategy{Method: "unsupported"}, errors.New("TVDb search requires TV media")
		}
		query := idQuery(m, override)
		if override != "title" && query != "" {
			omit := m.Type == "tv" && len(idx.Capabilities) > 0 && ((m.Season != nil && !supports(idx, "tv-search-season")) || (m.Episode != nil && !supports(idx, "tv-search-ep")))
			return models.SearchStrategy{Method: override, Query: query, OmitTVParameters: omit, Reason: "Indexer search override"}, nil
		}
		if m.Title != "" {
			reason := "Indexer search override"
			if override != "title" {
				reason = fmt.Sprintf("%s ID unavailable; using title search", strings.ToUpper(override))
			}
			return models.SearchStrategy{Method: "title", Query: titleQuery(m), Reason: reason}, nil
		}
		return models.SearchStrategy{Method: "unsupported"}, fmt.Errorf("indexer override requires %s or a resolved title", override)
	}
	unknown := len(idx.Capabilities) == 0
	structured := unknown || ((m.Season == nil || supports(idx, "tv-search-season")) && (m.Episode == nil || supports(idx, "tv-search-ep")))
	for _, method := range methods {
		query := idQuery(m, method)
		if query != "" && structured && (unknown || supports(idx, prefix+method+"id")) {
			return models.SearchStrategy{Method: method, Query: query}, nil
		}
	}
	if m.Title != "" && (unknown || supports(idx, prefix+"q") || supports(idx, "search")) {
		query := titleQuery(m)
		return models.SearchStrategy{Method: "title", Query: query}, nil
	}
	return models.SearchStrategy{Method: "unsupported"}, errors.New("indexer does not advertise a supported search for the available IDs and TV parameters")
}

func idQuery(m models.Media, method string) string {
	switch method {
	case "imdb":
		return m.IMDbID
	case "tmdb":
		if m.TMDbID > 0 {
			return strconv.Itoa(m.TMDbID)
		}
	case "tvdb":
		if m.TVDbID > 0 {
			return strconv.Itoa(m.TVDbID)
		}
	}
	return ""
}
func titleQuery(m models.Media) string {
	query := m.Title
	if m.Year > 0 {
		query += fmt.Sprintf(" %d", m.Year)
	}
	if m.Season != nil {
		query += fmt.Sprintf(" S%02d", *m.Season)
	}
	if m.Episode != nil {
		query += fmt.Sprintf("E%02d", *m.Episode)
	}
	return query
}
