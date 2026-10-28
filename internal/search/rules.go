package search

import (
	"ravenseed/internal/models"
	"ravenseed/internal/releases"
)

func (s *Service) SaveMatching(input models.MatchingRules) error {
	rules, err := releases.NormalizeRules(&input)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.settings
	v.Matching = &rules
	if err = s.persistSettings(v); err != nil {
		return internal(err)
	}
	s.settings = v
	return nil
}
