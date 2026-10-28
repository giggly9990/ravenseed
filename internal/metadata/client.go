package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"ravenseed/internal/httpx"
	"ravenseed/internal/models"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var imdbPattern = regexp.MustCompile(`^tt[0-9]{7,10}$`)
var languagePattern = regexp.MustCompile(`^[a-z]{2}(?:-[A-Z]{2})?$`)
var imagePattern = regexp.MustCompile(`^/[a-zA-Z0-9_-]+\.(?:jpg|png|webp)$`)
var apiKeyPattern = regexp.MustCompile(`^[a-fA-F0-9]{32}$`)

func ParseInput(input models.ResolveRequest) (models.Media, error) {
	m := models.Media{Type: input.Type, Season: input.Season}
	if m.Type != "" && m.Type != "movie" && m.Type != "tv" {
		return m, errors.New("type must be movie or tv")
	}
	if m.Season != nil && (m.Type == "movie" || *m.Season < 0 || *m.Season > 999) {
		return m, errors.New("season requires TV and a value from 0 to 999")
	}
	if m.Season != nil {
		m.Type = "tv"
	}
	id := strings.ToLower(strings.TrimSpace(input.ID))
	if strings.HasPrefix(id, "imdb:") {
		id = strings.TrimPrefix(id, "imdb:")
		if !imdbPattern.MatchString(id) {
			return m, errors.New("invalid IMDb ID")
		}
	}
	if strings.HasPrefix(id, "tmdb:") {
		id = strings.TrimPrefix(id, "tmdb:")
		if strings.HasPrefix(id, "tt") {
			return m, errors.New("invalid TMDb ID")
		}
	}
	if imdbPattern.MatchString(id) {
		m.IMDbID = id
		return m, nil
	}
	n, err := strconv.Atoi(id)
	if err != nil || n <= 0 || n > 2147483647 || strings.Trim(id, "0123456789") != "" {
		return m, errors.New("enter an IMDb or TMDb ID")
	}
	m.TMDbID = n
	if m.Type == "" {
		m.Type = "movie"
	}
	return m, nil
}

func Language(s string) (string, error) {
	if s == "" {
		return "en-US", nil
	}
	if !languagePattern.MatchString(s) {
		return "", errors.New("use a language such as en-US or fr")
	}
	return s, nil
}

type Client struct {
	base, token, language string
	http                  *http.Client
}

func New(base, token, language string) (*Client, error) {
	if base == "" {
		base = "https://api.themoviedb.org/3"
	}
	if !httpx.ValidBaseURL(base) {
		return nil, errors.New("invalid TMDb endpoint")
	}
	lang, err := Language(language)
	if err != nil {
		return nil, err
	}
	token = strings.TrimSpace(token)
	if len(token) >= 7 && strings.EqualFold(token[:7], "Bearer ") {
		token = strings.TrimSpace(token[7:])
	}
	if token == "" {
		return nil, errors.New("TMDb is not configured; direct-ID search remains available")
	}
	return &Client{strings.TrimRight(base, "/"), strings.TrimSpace(token), lang, httpx.NewClient(8 * time.Second)}, nil
}

func (c *Client) get(ctx context.Context, path string, params url.Values, out any) (json.RawMessage, error) {
	if params == nil {
		params = url.Values{}
	}
	params.Set("language", c.language)
	apiKey := apiKeyPattern.MatchString(c.token)
	if apiKey {
		params.Set("api_key", c.token)
	}
	req, err := http.NewRequestWithContext(ctx, "GET", c.base+path+"?"+params.Encode(), nil)
	if err != nil {
		return nil, errors.New("invalid TMDb request")
	}
	if !apiKey {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("Accept", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return nil, httpx.RequestError(ctx, errors.New("TMDb connection failed or timed out"))
	}
	defer httpx.Close(res.Body)
	if res.StatusCode == http.StatusUnauthorized {
		return nil, errors.New("TMDb rejected the credential (HTTP 401). Check your API key or API Read Access Token in TMDb account settings")
	}
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("TMDb returned HTTP %d", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil || json.Unmarshal(b, out) != nil {
		return nil, errors.New("invalid TMDb response")
	}
	return b, nil
}

func (c *Client) Test(ctx context.Context) error {
	var out map[string]any
	_, err := c.get(ctx, "/configuration", nil, &out)
	return err
}

func (c *Client) Resolve(ctx context.Context, m models.Media) (models.MediaSnapshot, json.RawMessage, error) {
	var out models.MediaSnapshot
	if m.TMDbID == 0 {
		var found struct {
			Movies []struct {
				ID int `json:"id"`
			} `json:"movie_results"`
			TV []struct {
				ID int `json:"id"`
			} `json:"tv_results"`
		}
		if _, err := c.get(ctx, "/find/"+m.IMDbID, url.Values{"external_source": {"imdb_id"}}, &found); err != nil {
			return out, nil, err
		}
		if m.Type == "" {
			if len(found.Movies) == 1 && len(found.TV) == 0 {
				m.Type = "movie"
			} else if len(found.TV) == 1 && len(found.Movies) == 0 {
				m.Type = "tv"
			} else {
				return out, nil, errors.New("media identity is ambiguous or unavailable; select movie or TV")
			}
		}
		if m.Type == "movie" && len(found.Movies) == 1 {
			m.TMDbID = found.Movies[0].ID
		} else if m.Type == "tv" && len(found.TV) == 1 {
			m.TMDbID = found.TV[0].ID
		} else {
			return out, nil, errors.New("TMDb found no unique match for this media type")
		}
	}
	var detail struct {
		ID            int    `json:"id"`
		Title         string `json:"title"`
		Name          string `json:"name"`
		OriginalTitle string `json:"original_title"`
		OriginalName  string `json:"original_name"`
		Date          string `json:"release_date"`
		FirstAir      string `json:"first_air_date"`
		Overview      string `json:"overview"`
		Poster        string `json:"poster_path"`
		Backdrop      string `json:"backdrop_path"`
		IMDb          string `json:"imdb_id"`
		Runtime       int    `json:"runtime"`
		EpisodeRun    []int  `json:"episode_run_time"`
		External      struct {
			IMDb string `json:"imdb_id"`
			TVDb int    `json:"tvdb_id"`
		} `json:"external_ids"`
		Seasons []struct {
			Number   int    `json:"season_number"`
			Name     string `json:"name"`
			Air      string `json:"air_date"`
			Episodes int    `json:"episode_count"`
			Overview string `json:"overview"`
			Poster   string `json:"poster_path"`
		} `json:"seasons"`
	}
	raw, err := c.get(ctx, fmt.Sprintf("/%s/%d", m.Type, m.TMDbID), url.Values{"append_to_response": {"external_ids"}}, &detail)
	if err != nil {
		return out, nil, err
	}
	if detail.ID != m.TMDbID {
		return out, nil, errors.New("TMDb returned a different media identity")
	}
	id := detail.External.IMDb
	if id == "" {
		id = detail.IMDb
	}
	if m.IMDbID != "" && id != "" && m.IMDbID != id {
		return out, nil, errors.New("TMDb returned conflicting external IDs")
	}
	if id != "" {
		m.IMDbID = id
	}
	m.TVDbID = detail.External.TVDb
	m.Title, m.OriginalTitle = detail.Title, detail.OriginalTitle
	date := detail.Date
	if m.Type == "tv" {
		m.Title, m.OriginalTitle, date = detail.Name, detail.OriginalName, detail.FirstAir
	}
	if m.Title == "" {
		return out, nil, errors.New("TMDb returned no title")
	}
	if len(date) >= 4 {
		m.Year, _ = strconv.Atoi(date[:4])
	}
	m.Season, m.Episode = nil, nil
	now := time.Now().UTC()
	out = models.MediaSnapshot{Media: m, Overview: detail.Overview, PosterPath: imagePath(detail.Poster), BackdropPath: imagePath(detail.Backdrop), FetchedAt: now, ExpiresAt: now.Add(7 * 24 * time.Hour), Runtime: max(detail.Runtime, 0)}
	for _, minutes := range detail.EpisodeRun {
		if out.Runtime == 0 && minutes > 0 {
			out.Runtime = minutes
		}
	}
	for _, season := range detail.Seasons {
		out.Seasons = append(out.Seasons, models.SeasonMetadata{Number: season.Number, Name: season.Name, AirDate: season.Air, EpisodeCount: season.Episodes, Overview: season.Overview, PosterPath: imagePath(season.Poster)})
	}
	return out, raw, nil
}

func (c *Client) Season(ctx context.Context, id, number int) (models.SeasonMetadata, json.RawMessage, error) {
	var out models.SeasonMetadata
	var detail struct {
		Number   int               `json:"season_number"`
		Name     string            `json:"name"`
		Air      string            `json:"air_date"`
		Overview string            `json:"overview"`
		Poster   string            `json:"poster_path"`
		Episodes []json.RawMessage `json:"episodes"`
	}
	raw, err := c.get(ctx, fmt.Sprintf("/tv/%d/season/%d", id, number), nil, &detail)
	if err != nil {
		return out, nil, err
	}
	if detail.Number != number {
		return out, nil, errors.New("TMDb returned a different season")
	}
	return models.SeasonMetadata{Number: number, Name: detail.Name, AirDate: detail.Air, EpisodeCount: len(detail.Episodes), Overview: detail.Overview, PosterPath: imagePath(detail.Poster)}, raw, nil
}

func imagePath(path string) string {
	if imagePattern.MatchString(path) {
		return path
	}
	return ""
}
