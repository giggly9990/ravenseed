package qui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"ravenseed/internal/models"
	"slices"
	"strconv"
	"strings"
)

func (c *Client) Instances(ctx context.Context) ([]models.QuiInstance, error) {
	out := []models.QuiInstance{}
	err := c.do(ctx, http.MethodGet, "/api/instances", nil, &out)
	return out, err
}

func (c *Client) Categories(ctx context.Context, instanceID int) ([]string, error) {
	if instanceID <= 0 {
		return nil, errors.New("instance ID must be positive")
	}
	var categories map[string]json.RawMessage
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api/instances/%d/categories", instanceID), nil, &categories); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(categories))
	for name := range categories {
		if name != "" {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names, nil
}

// ErrNoDownloadLink reports a result qui cannot add: its link is not an
// HTTP(S) or magnet link, or it has no indexer.
var ErrNoDownloadLink = errors.New("this result has no usable torrent download link")

// CheckDownloadLink reports ErrNoDownloadLink for a result qui cannot add.
func CheckDownloadLink(result models.Result) error {
	u, err := url.Parse(result.DownloadURL)
	if err != nil || strings.ContainsAny(result.DownloadURL, "\r\n,") || result.IndexerID <= 0 {
		return ErrNoDownloadLink
	}
	if u.Scheme != "magnet" && ((u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil) {
		return ErrNoDownloadLink
	}
	return nil
}

func (c *Client) AddTorrent(ctx context.Context, result models.Result, input models.AddTorrentRequest) (models.AddTorrentResponse, error) {
	var out models.AddTorrentResponse
	if err := CheckDownloadLink(result); err != nil || input.InstanceID <= 0 {
		return out, ErrNoDownloadLink
	}
	paused := true
	if input.Paused != nil {
		paused = *input.Paused
	}
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	for key, value := range map[string]string{"urls": result.DownloadURL, "indexer_id": strconv.Itoa(result.IndexerID), "paused": strconv.FormatBool(paused), "category": input.Category, "savepath": input.SavePath, "tags": input.Tags} {
		if err := form.WriteField(key, value); err != nil {
			return out, err
		}
	}
	if err := form.Close(); err != nil {
		return out, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/api/instances/%d/torrents", c.base, input.InstanceID), &body)
	if err != nil {
		return out, errors.New("invalid qui request")
	}
	req.Header.Set("Content-Type", form.FormDataContentType())
	if err = c.send(req, &out); err != nil {
		return out, fmt.Errorf("%w; check qui before retrying", err)
	}
	if out.Added != 1 || out.Failed != 0 {
		return out, errors.New("qui did not confirm the torrent was added; check qui before retrying")
	}
	return out, nil
}
