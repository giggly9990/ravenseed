package search

import (
	"fmt"
	"ravenseed/internal/models"
	"slices"
	"strings"
)

// Attach a settings snapshot without changing the cached upstream capabilities.
func withSearchOverrides(indexers []models.Indexer, overrides map[int]models.IndexerSearchOverride) []models.Indexer {
	out := slices.Clone(indexers)
	for i := range out {
		out[i].SearchOverride = nil
		if v, ok := overrides[out[i].ID]; ok {
			out[i].SearchOverride = &v
		}
	}
	return out
}

func (s *Service) SaveSearchOverrides(input map[int]models.IndexerSearchOverride) error {
	if len(input) > 1000 {
		return fmt.Errorf("too many indexer search overrides")
	}
	normalized := make(map[int]models.IndexerSearchOverride)
	for id, v := range input {
		if id <= 0 {
			return fmt.Errorf("indexer ID must be positive")
		}
		movie, tv := strings.ToLower(strings.TrimSpace(v.Movie)), strings.ToLower(strings.TrimSpace(v.TV))
		if movie == "" {
			movie = "auto"
		}
		if tv == "" {
			tv = "auto"
		}
		if !slices.Contains([]string{"auto", "imdb", "tmdb", "title"}, movie) || !slices.Contains([]string{"auto", "imdb", "tmdb", "tvdb", "title"}, tv) {
			return fmt.Errorf("invalid search method for indexer %d", id)
		}
		if movie != "auto" || tv != "auto" {
			normalized[id] = models.IndexerSearchOverride{Movie: movie, TV: tv}
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.settings
	v.SearchOverrides = normalized
	if err := s.persistSettings(v); err != nil {
		return internal(err)
	}
	s.settings = v
	return nil
}
