package qui

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ravenseed/internal/httpx"
	"ravenseed/internal/models"
	"ravenseed/internal/releases"
)

type Client struct {
	base, key string
	http      *http.Client
}

func New(base, key string, timeout time.Duration) (*Client, error) {
	if !httpx.ValidBaseURL(base) {
		return nil, errors.New("qui URL must be an HTTP or HTTPS base URL without credentials or query parameters")
	}
	if strings.TrimSpace(key) == "" {
		return nil, errors.New("qui API key is required")
	}
	return &Client{strings.TrimRight(base, "/"), key, httpx.NewClient(timeout)}, nil
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return errors.New("invalid qui request")
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.send(req, out)
}

func (c *Client) send(req *http.Request, out any) error {
	req.Header.Set("X-API-Key", c.key)
	req.Header.Set("Accept", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return httpx.RequestError(req.Context(), errors.New("qui connection failed or timed out"))
	}
	defer httpx.Close(res.Body)
	if httpx.AuthRejected(res.StatusCode) {
		return errors.New("qui rejected the API key")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return errors.New("qui returned an error response")
	}
	if err = httpx.DecodeJSON(res.Body, 32<<20, out); err != nil {
		return errors.New("qui returned an invalid or oversized response")
	}
	return nil
}

func (c *Client) Indexers(ctx context.Context) ([]models.Indexer, error) {
	out := []models.Indexer{}
	err := c.do(ctx, "GET", "/api/torznab/indexers", nil, &out)
	return out, err
}

func (c *Client) Search(ctx context.Context, req models.SearchRequest, idx models.Indexer) ([]models.Result, error) {
	strategy, err := Strategy(req.Media, idx)
	if err != nil {
		return nil, err
	}
	category := 2000
	if req.Media.Type == "tv" {
		category = 5000
	}
	body := map[string]any{"indexer_ids": []int{idx.ID}, "categories": []int{category}, "limit": 1000, "offset": 0}
	switch strategy.Method {
	case "imdb":
		body["imdb_id"] = req.Media.IMDbID
	case "tmdb":
		body["tmdb_id"] = req.Media.TMDbID
	case "tvdb":
		body["tvdb_id"] = strconv.Itoa(req.Media.TVDbID)
	case "title":
		body["query"] = strategy.Query
	}
	if req.Media.Year > 0 {
		body["year"] = req.Media.Year
	}
	if req.Media.Season != nil && strategy.Method != "title" && !strategy.OmitTVParameters {
		body["season"] = *req.Media.Season
	}
	if req.Media.Episode != nil && strategy.Method != "title" && !strategy.OmitTVParameters {
		body["episode"] = *req.Media.Episode
	}
	if req.Refresh {
		body["cache_mode"] = "bypass"
	}
	out := []models.Result{}
	seen := map[string]bool{}
	for offset := 0; offset < 100000; {
		body["offset"] = offset
		var page struct {
			Results []json.RawMessage `json:"results"`
			Total   int               `json:"total"`
			Partial bool              `json:"partial"`
		}
		if err := c.do(ctx, "POST", "/api/torznab/search", body, &page); err != nil {
			return out, err
		}
		for _, raw := range page.Results {
			var wire struct {
				IndexerID      int             `json:"indexer_id"`
				Title          string          `json:"title"`
				Size           int64           `json:"size"`
				Seeders        int             `json:"seeders"`
				Leechers       int             `json:"leechers"`
				CategoryID     int             `json:"category_id"`
				CategoryName   string          `json:"category_name"`
				DownloadURL    string          `json:"download_url"`
				InfoURL        string          `json:"info_url"`
				PublishDate    time.Time       `json:"publish_date"`
				DownloadFactor *float64        `json:"download_volume_factor"`
				UploadFactor   *float64        `json:"upload_volume_factor"`
				GUID           string          `json:"guid"`
				IMDbID         string          `json:"imdb_id"`
				TMDbID         json.RawMessage `json:"tmdb_id"`
				SearchIMDbID   string          `json:"search_imdb_id"`
				SearchTMDbID   json.RawMessage `json:"search_tmdb_id"`
				TVDbID         json.RawMessage `json:"tvdb_id"`
				SearchTVDbID   json.RawMessage `json:"search_tvdb_id"`
			}
			if err := json.Unmarshal(raw, &wire); err != nil {
				return out, errors.New("qui returned invalid result metadata")
			}
			if wire.IndexerID != 0 && wire.IndexerID != idx.ID {
				return out, errors.New("qui returned an unexpected indexer")
			}
			if strings.TrimSpace(wire.Title) == "" {
				continue
			}
			tmdb := strings.Trim(string(wire.TMDbID), "\"")
			tvdb := strings.Trim(string(wire.TVDbID), "\"")
			searchTVDb := strings.Trim(string(wire.SearchTVDbID), "\"")
			searchTMDb := strings.Trim(string(wire.SearchTMDbID), "\"")
			if req.Media.IMDbID != "" && wire.IMDbID != "" && normalizeIMDb(wire.IMDbID) != req.Media.IMDbID {
				continue
			}
			if req.Media.TMDbID > 0 && tmdb != "" && tmdb != "null" && tmdb != "0" && tmdb != strconv.Itoa(req.Media.TMDbID) {
				continue
			}
			var identityEvidence []string
			if req.Media.TVDbID > 0 && tvdb != "" && tvdb != "null" && tvdb != "0" && tvdb != strconv.Itoa(req.Media.TVDbID) {
				// Trackers can retain a retired TVDb ID after the series is recreated.
				// Only two actual matching primary IDs can corroborate this exception;
				// echoed search IDs and title similarity are not sufficient.
				corroborated := req.Media.Type == "tv" && req.Media.IMDbID != "" && req.Media.TMDbID > 0 && normalizeIMDb(wire.IMDbID) == req.Media.IMDbID && tmdb == strconv.Itoa(req.Media.TMDbID)
				if !corroborated {
					continue
				}
				identityEvidence = []string{fmt.Sprintf("TVDb ID differs: tracker %s; resolved %d. Accepted because returned IMDb and TMDb IDs both match.", tvdb, req.Media.TVDbID)}
			}
			verified := (req.Media.IMDbID != "" && (normalizeIMDb(wire.IMDbID) == req.Media.IMDbID || (strategy.Method == "imdb" && normalizeIMDb(wire.SearchIMDbID) == req.Media.IMDbID))) || (req.Media.TMDbID > 0 && (tmdb == strconv.Itoa(req.Media.TMDbID) || (strategy.Method == "tmdb" && searchTMDb == strconv.Itoa(req.Media.TMDbID)))) || (req.Media.TVDbID > 0 && (tvdb == strconv.Itoa(req.Media.TVDbID) || (strategy.Method == "tvdb" && searchTVDb == strconv.Itoa(req.Media.TVDbID))))
			h := sha256.Sum256([]byte(fmt.Sprintf("%d|%s|%s|%d|%s", idx.ID, wire.GUID, wire.Title, wire.Size, wire.DownloadURL)))
			id := hex.EncodeToString(h[:12])
			if seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, models.Result{ID: id, IndexerID: idx.ID, Indexer: idx.Name, Title: wire.Title, Size: wire.Size, Seeders: wire.Seeders, Leechers: wire.Leechers, CategoryID: wire.CategoryID, CategoryName: wire.CategoryName, DownloadURL: wire.DownloadURL, InfoURL: wire.InfoURL, PublishDate: wire.PublishDate, DownloadFactor: wire.DownloadFactor, UploadFactor: wire.UploadFactor, Raw: raw, IdentityVerified: verified, IdentityEvidence: identityEvidence})
		}
		if page.Partial {
			return out, errors.New("qui reported incomplete indexer coverage")
		}
		offset += len(page.Results)
		if offset >= page.Total {
			return out, nil
		}
		if len(page.Results) == 0 {
			return out, errors.New("qui pagination ended before all results were returned")
		}
	}
	return out, errors.New("qui result limit exceeded")
}

// normalizeIMDb returns "" for a missing or malformed ID, which never equals
// a requested one.
func normalizeIMDb(s string) string { return releases.NormalizeIMDbID(s) }
