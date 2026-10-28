package qui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

type Torrent struct {
	Hash        string  `json:"hash"`
	Name        string  `json:"name"`
	Tracker     string  `json:"tracker"`
	Size        int64   `json:"size"`
	TotalSize   int64   `json:"total_size"`
	Tags        string  `json:"tags"`
	Category    string  `json:"category"`
	State       string  `json:"state"`
	Progress    float64 `json:"progress"`
	AmountLeft  int64   `json:"amount_left"`
	ContentPath string  `json:"content_path"`
}

var ErrTorrentNotFound = errors.New("torrent not found in qui")

// Torrents reads the complete inventory for one instance in bounded pages.
func (c *Client) Torrents(ctx context.Context, instance int) ([]Torrent, error) {
	if instance <= 0 {
		return nil, fmt.Errorf("instance ID must be positive")
	}
	out := []Torrent{}
	for page := 0; page < 100; page++ {
		q := url.Values{"limit": {"1000"}, "page": {fmt.Sprint(page)}}
		var response struct {
			Torrents []Torrent `json:"torrents"`
			Total    int       `json:"total"`
			HasMore  bool      `json:"hasMore"`
		}
		if err := c.do(ctx, "GET", fmt.Sprintf("/api/instances/%d/torrents?%s", instance, q.Encode()), nil, &response); err != nil {
			return nil, err
		}
		out = append(out, response.Torrents...)
		if len(response.Torrents) == 0 || (!response.HasMore && (len(response.Torrents) < 1000 || (response.Total > 0 && len(out) >= response.Total))) {
			return out, nil
		}
	}
	return nil, fmt.Errorf("qui torrent inventory exceeds 100000 items")
}

func (c *Client) TorrentByHash(ctx context.Context, instance int, hash string) (Torrent, error) {
	filters, _ := json.Marshal(map[string]any{"hashes": []string{hash}})
	q := url.Values{"filters": {string(filters)}, "limit": {"100"}, "page": {"0"}}
	var response struct {
		Torrents []Torrent `json:"torrents"`
	}
	if err := c.do(ctx, "GET", fmt.Sprintf("/api/instances/%d/torrents?%s", instance, q.Encode()), nil, &response); err != nil {
		return Torrent{}, err
	}
	for _, item := range response.Torrents {
		if strings.EqualFold(item.Hash, hash) {
			return item, nil
		}
	}
	return Torrent{}, ErrTorrentNotFound
}

func (c *Client) TaggedTorrents(ctx context.Context, instance int, tag string) ([]Torrent, error) {
	filters, _ := json.Marshal(map[string]any{"tags": []string{tag}})
	q := url.Values{"filters": {string(filters)}, "limit": {"100"}, "page": {"0"}}
	var out struct {
		Torrents []Torrent `json:"torrents"`
	}
	err := c.do(ctx, "GET", fmt.Sprintf("/api/instances/%d/torrents?%s", instance, q.Encode()), nil, &out)
	return out.Torrents, err
}

func (c *Client) FilesComplete(ctx context.Context, instance int, hash string) (bool, error) {
	var files []struct {
		Size     int64   `json:"size"`
		Progress float64 `json:"progress"`
	}
	err := c.do(ctx, "GET", fmt.Sprintf("/api/instances/%d/torrents/%s/files", instance, url.PathEscape(hash)), nil, &files)
	if err != nil {
		return false, err
	}
	if len(files) == 0 {
		return false, nil
	}
	for _, f := range files {
		if f.Size > 0 && f.Progress < 1 {
			return false, nil
		}
	}
	return true, nil
}

// TorrentFile is one file inside a torrent, by its path in the torrent.
type TorrentFile struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

func (c *Client) Files(ctx context.Context, instance int, hash string) ([]TorrentFile, error) {
	var files []TorrentFile
	err := c.do(ctx, "GET", fmt.Sprintf("/api/instances/%d/torrents/%s/files", instance, url.PathEscape(hash)), nil, &files)
	return files, err
}
