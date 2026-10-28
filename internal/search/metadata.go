package search

import (
	"context"
	"errors"
	"fmt"
	"ravenseed/internal/metadata"
	"ravenseed/internal/models"
	"strings"
	"time"
)

func (s *Service) persistSettings(v models.Settings) error {
	if s.Config.APIKey != "" {
		v.APIKey = ""
	}
	if s.Config.QuiURL != "" {
		v.QuiURL = ""
	}
	if v.Metadata != nil {
		copy := *v.Metadata
		if s.Config.TMDbToken != "" {
			copy.Token = ""
		}
		v.Metadata = &copy
	}
	return s.Store.SaveSettings(v)
}

func (s *Service) metadataSettings(input models.MetadataSettings) (models.MetadataSettings, error) {
	s.mu.RLock()
	current := *s.settings.Metadata
	s.mu.RUnlock()
	if current.EnvironmentManaged && (input.Token != "" || input.ClearToken) {
		return current, errors.New("TMDb token is managed by environment configuration")
	}
	if input.Token != "" && input.ClearToken {
		return current, errors.New("cannot set and clear the TMDb token together")
	}
	if len(input.Token) > 4096 || strings.ContainsAny(input.Token, "\r\n") {
		return current, errors.New("invalid TMDb token")
	}
	var err error
	input.Language, err = metadata.Language(input.Language)
	if err != nil {
		return current, err
	}
	if input.ClearToken {
		input.Token = ""
	} else if input.Token == "" {
		input.Token = current.Token
	}
	input.EnvironmentManaged = current.EnvironmentManaged
	input.ClearToken = false
	return input, nil
}

func (s *Service) TestMetadata(ctx context.Context, input models.MetadataSettings) error {
	input, err := s.metadataSettings(input)
	if err != nil {
		return err
	}
	c, err := metadata.New(s.Config.TMDbBaseURL, input.Token, input.Language)
	if err != nil {
		return err
	}
	return upstream(c.Test(ctx))
}

func (s *Service) SaveMetadataSettings(ctx context.Context, input models.MetadataSettings) error {
	input, err := s.metadataSettings(input)
	if err != nil {
		return err
	}
	if input.Token != "" {
		c, e := metadata.New(s.Config.TMDbBaseURL, input.Token, input.Language)
		if e != nil {
			return e
		}
		if e = c.Test(ctx); e != nil {
			return upstream(e)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.settings
	v.Metadata = &input
	if err = s.persistSettings(v); err != nil {
		return internal(err)
	}
	s.settings = v
	return nil
}

func metadataKey(m models.Media, language string) string {
	id := fmt.Sprintf("tmdb:%d", m.TMDbID)
	if m.IMDbID != "" {
		id = "imdb:" + m.IMDbID
	}
	return m.Type + "/" + id + "/" + language
}

func (s *Service) Resolve(ctx context.Context, input models.ResolveRequest) (models.MediaSnapshot, error) {
	m, err := metadata.ParseInput(input)
	if err != nil {
		return models.MediaSnapshot{}, err
	}
	s.mu.RLock()
	settings := *s.settings.Metadata
	s.mu.RUnlock()
	key := metadataKey(m, settings.Language)
	var cached models.MediaSnapshot
	expires, cacheErr := s.Store.Metadata(key, false, &cached)
	var out models.MediaSnapshot
	c, clientErr := metadata.New(s.Config.TMDbBaseURL, settings.Token, settings.Language)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if cacheErr == nil && time.Now().Before(expires) && !input.Refresh {
		out = cached
		out.Cached = true
	} else {
		err = clientErr
		if err == nil {
			var raw []byte
			out, raw, err = c.Resolve(ctx, m)
			if err == nil {
				keys := []string{key, metadataKey(models.Media{Type: out.Type, TMDbID: out.TMDbID}, settings.Language)}
				if out.IMDbID != "" {
					keys = append(keys, metadataKey(models.Media{Type: out.Type, IMDbID: out.IMDbID}, settings.Language))
				}
				for _, k := range keys {
					if e := s.Store.SaveMetadata(k, false, out, raw, out.FetchedAt, out.ExpiresAt); e != nil {
						out.Warning = "Metadata could not be cached"
					}
				}
			}
		}
		if err != nil {
			if cacheErr == nil {
				out = cached
				out.Cached = true
				out.Warning = "Using stale metadata. " + err.Error()
			} else {
				if m.Type == "" {
					m.Type = "movie"
				}
				out = models.MediaSnapshot{Media: m, Warning: err.Error() + ". Search will use the entered ID."}
			}
		}
	}
	out.Season = m.Season
	if out.Type == "tv" && m.Season != nil && out.TMDbID > 0 {
		seasonKey := fmt.Sprintf("%d/%d/%s", out.TMDbID, *m.Season, settings.Language)
		var season models.SeasonMetadata
		until, e := s.Store.Metadata(seasonKey, true, &season)
		if e == nil && time.Now().Before(until) && !input.Refresh {
			out.SeasonDetails = &season
		} else if clientErr == nil {
			fresh, raw, e := c.Season(ctx, out.TMDbID, *m.Season)
			if e == nil {
				out.SeasonDetails = &fresh
				now := time.Now().UTC()
				if e = s.Store.SaveMetadata(seasonKey, true, fresh, raw, now, now.Add(7*24*time.Hour)); e != nil {
					out.Warning = "Season metadata could not be cached"
				}
			} else {
				out.Warning = "Season metadata unavailable; using series metadata. " + e.Error()
			}
		} else {
			out.Warning = "Season metadata unavailable; using series metadata"
		}
	}
	return out, nil
}
