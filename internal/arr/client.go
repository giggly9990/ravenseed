// Package arr reads media identities from Sonarr and Radarr. It only issues
// GET requests and never changes the instances it reads.
package arr

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
	"ravenseed/internal/releases"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	Sonarr = "sonarr"
	Radarr = "radarr"
)

// Match is a media identity reported by one instance.
type Match struct {
	Media    models.Media
	Episodes []models.EpisodeMapping
	// Resolution is the quality recorded in download history, such as "720p".
	Resolution string
	// DownloadID is the torrent hash a download-history match was found for.
	DownloadID string
	// ItemID is the movie or series ID in the instance library, when known.
	ItemID int
	// Runtime is the movie or episode runtime in minutes; SeasonEpisodes
	// counts episodes per season when the instance reports them.
	Runtime        int
	SeasonEpisodes map[int]int
}

// withFacts adds the runtime and episode counts the item reports.
func (m *Match) withFacts(v item) *Match {
	m.Runtime = max(v.Runtime, 0)
	for _, s := range v.Seasons {
		if s.Statistics != nil && s.Statistics.TotalEpisodeCount > 0 {
			if m.SeasonEpisodes == nil {
				m.SeasonEpisodes = map[int]int{}
			}
			m.SeasonEpisodes[s.SeasonNumber] = s.Statistics.TotalEpisodeCount
		}
	}
	return m
}

// ImportedFile is one file Sonarr or Radarr imported from a download. The
// import record keeps the downloaded file's name and exact size after the
// torrent itself is gone.
type ImportedFile struct {
	Name       string
	Size       int64
	DownloadID string
	Resolution string
	Episode    *models.EpisodeMapping
}

type Client struct {
	base, key, kind string
	http            *http.Client
}

func New(base, key, kind string, timeout time.Duration) (*Client, error) {
	if kind != Sonarr && kind != Radarr {
		return nil, errors.New("instance type must be sonarr or radarr")
	}
	if err := validURL(base); err != nil {
		return nil, err
	}
	if strings.TrimSpace(key) == "" {
		return nil, errors.New("API key is required")
	}
	return &Client{strings.TrimRight(base, "/"), key, kind, httpx.NewClient(timeout)}, nil
}

func validURL(base string) error {
	if !httpx.ValidBaseURL(base) {
		return errors.New("use an HTTP or HTTPS URL without credentials or query parameters")
	}
	return nil
}

// Validate normalizes saved instances. Missing IDs are left for the caller.
func Validate(v models.ArrSettings) (models.ArrSettings, error) {
	if v.Instances == nil {
		v.Instances = []models.ArrInstance{}
	}
	if len(v.Instances) > 10 {
		return v, errors.New("configure at most 10 Sonarr or Radarr instances")
	}
	ids, names := map[string]bool{}, map[string]bool{}
	for i := range v.Instances {
		x := &v.Instances[i]
		x.Name = strings.TrimSpace(x.Name)
		x.Kind = strings.ToLower(strings.TrimSpace(x.Kind))
		x.URL = strings.TrimRight(strings.TrimSpace(x.URL), "/")
		x.APIKey = strings.TrimSpace(x.APIKey)
		if x.Kind != Sonarr && x.Kind != Radarr {
			return v, errors.New("instance type must be sonarr or radarr")
		}
		if x.Name == "" || len(x.Name) > 80 || strings.ContainsFunc(x.Name, unicode.IsControl) {
			return v, errors.New("give each instance a name of up to 80 characters")
		}
		// Identities record the instance by name, so names must be unique.
		if names[strings.ToLower(x.Name)] {
			return v, fmt.Errorf("%s: instance names must be unique", x.Name)
		}
		names[strings.ToLower(x.Name)] = true
		if err := validURL(x.URL); err != nil {
			return v, fmt.Errorf("%s: %w", x.Name, err)
		}
		if len(x.APIKey) > 256 || strings.ContainsFunc(x.APIKey, unicode.IsControl) {
			return v, fmt.Errorf("%s: invalid API key", x.Name)
		}
		if x.Enabled && x.APIKey == "" {
			return v, fmt.Errorf("%s: enter an API key before enabling it", x.Name)
		}
		if x.ID != "" {
			if ids[x.ID] || len(x.ID) > 64 {
				return v, errors.New("instance IDs must be unique")
			}
			ids[x.ID] = true
		}
		x.HasAPIKey = false
	}
	return v, nil
}

func (c *Client) get(ctx context.Context, path string, params url.Values, out any) error {
	target := c.base + path
	if len(params) > 0 {
		target += "?" + params.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return errors.New("invalid request")
	}
	req.Header.Set("X-Api-Key", c.key)
	req.Header.Set("Accept", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return httpx.RequestError(ctx, fmt.Errorf("%s connection failed or timed out", c.kind))
	}
	defer httpx.Close(res.Body)
	if httpx.AuthRejected(res.StatusCode) {
		return fmt.Errorf("%s rejected the API key", c.kind)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("%s returned an error response", c.kind)
	}
	if decode, ok := out.(func(io.Reader) error); ok {
		return decode(res.Body)
	}
	if err = httpx.DecodeJSON(res.Body, maxResponse, out); err != nil {
		return fmt.Errorf("%s returned an invalid or oversized response", c.kind)
	}
	return nil
}

// maxResponse bounds a single decoded response.
const maxResponse = 16 << 20

// Test confirms the URL is the configured kind of instance and the key works.
func (c *Client) Test(ctx context.Context) error {
	var status struct {
		AppName string `json:"appName"`
		Version string `json:"version"`
	}
	if err := c.get(ctx, "/api/v3/system/status", nil, &status); err != nil {
		return err
	}
	// The reply is not echoed: it would describe whatever answers the URL.
	if !strings.EqualFold(status.AppName, c.kind) {
		return fmt.Errorf("URL did not answer as %s", c.kind)
	}
	return nil
}

type item struct {
	ID            int    `json:"id"`
	Title         string `json:"title"`
	OriginalTitle string `json:"originalTitle"`
	Year          int    `json:"year"`
	IMDbID        string `json:"imdbId"`
	TMDbID        int    `json:"tmdbId"`
	TVDbID        int    `json:"tvdbId"`
	Runtime       int    `json:"runtime"`
	Seasons       []struct {
		SeasonNumber int `json:"seasonNumber"`
		Statistics   *struct {
			TotalEpisodeCount int `json:"totalEpisodeCount"`
		} `json:"statistics"`
	} `json:"seasons"`
	AlternateTitles []struct {
		Title string `json:"title"`
	} `json:"alternateTitles"`
}

type episode struct {
	SeasonNumber          int  `json:"seasonNumber"`
	EpisodeNumber         int  `json:"episodeNumber"`
	AbsoluteEpisodeNumber *int `json:"absoluteEpisodeNumber"`
}

func (c *Client) mediaType() string {
	if c.kind == Sonarr {
		return "tv"
	}
	return "movie"
}

func (c *Client) media(v item) models.Media {
	m := models.Media{Type: c.mediaType(), Title: v.Title, OriginalTitle: v.OriginalTitle, Year: v.Year, TMDbID: max(v.TMDbID, 0)}
	m.IMDbID = releases.NormalizeIMDbID(v.IMDbID)
	if c.kind == Sonarr {
		m.TVDbID = max(v.TVDbID, 0)
	}
	return m
}

func hasID(m models.Media) bool { return m.IMDbID != "" || m.TMDbID > 0 || m.TVDbID > 0 }

func episodeMap(m models.Media, episodes []episode) []models.EpisodeMapping {
	out := []models.EpisodeMapping{}
	for _, e := range episodes {
		if e.AbsoluteEpisodeNumber != nil && *e.AbsoluteEpisodeNumber > 0 && e.SeasonNumber > 0 && e.EpisodeNumber > 0 {
			mapping := models.EpisodeMapping{IMDbID: m.IMDbID, TMDbID: m.TMDbID, Absolute: *e.AbsoluteEpisodeNumber, Season: e.SeasonNumber, Episode: e.EpisodeNumber}
			// History repeats an episode for its grab and its import.
			if !slices.Contains(out, mapping) {
				out = append(out, mapping)
			}
		}
	}
	if len(out) == 0 || (m.IMDbID == "" && m.TMDbID == 0) {
		return nil
	}
	return out
}

// historyPageSize and historyPages bound the download history read for one
// torrent: a season pack records a grab and an import per episode.
const (
	historyPageSize = 250
	historyPages    = 4
)

type historyRecord struct {
	DownloadID string   `json:"downloadId"`
	SeriesID   int      `json:"seriesId"`
	MovieID    int      `json:"movieId"`
	Series     *item    `json:"series"`
	Movie      *item    `json:"movie"`
	Episode    *episode `json:"episode"`
	Quality    struct {
		Quality struct {
			Resolution int `json:"resolution"`
		} `json:"quality"`
	} `json:"quality"`
}

// History reads the download history recorded for a torrent hash. It is the
// only lookup bound to the torrent itself rather than to its name.
func (c *Client) History(ctx context.Context, hash string) (*Match, error) {
	params := url.Values{"downloadId": {strings.ToUpper(hash)}, "pageSize": {strconv.Itoa(historyPageSize)}, "sortKey": {"date"}, "sortDirection": {"descending"}}
	if c.kind == Sonarr {
		params.Set("includeSeries", "true")
		params.Set("includeEpisode", "true")
	} else {
		params.Set("includeMovie", "true")
	}
	records := []historyRecord{}
	for number := 1; number <= historyPages; number++ {
		params.Set("page", strconv.Itoa(number))
		var page struct {
			TotalRecords int             `json:"totalRecords"`
			Records      []historyRecord `json:"records"`
		}
		if err := c.get(ctx, "/api/v3/history", params, &page); err != nil {
			return nil, err
		}
		records = append(records, page.Records...)
		if len(page.Records) < historyPageSize || len(records) >= page.TotalRecords {
			break
		}
	}
	owner, embedded := 0, (*item)(nil)
	episodes := []episode{}
	resolution := 0
	for _, r := range records {
		// Older instances ignore the filter and return all history.
		if !strings.EqualFold(r.DownloadID, hash) {
			continue
		}
		id, v := r.MovieID, r.Movie
		if c.kind == Sonarr {
			id, v = r.SeriesID, r.Series
		}
		if id <= 0 {
			continue
		}
		if owner != 0 && owner != id {
			return nil, nil // one torrent, several titles: ambiguous
		}
		owner = id
		if v != nil {
			embedded = v
		}
		if r.Episode != nil {
			episodes = append(episodes, *r.Episode)
		}
		// Newest first: an import can correct the resolution of a grab.
		if resolution == 0 && r.Quality.Quality.Resolution > 0 {
			resolution = r.Quality.Quality.Resolution
		}
	}
	if owner == 0 {
		return nil, nil
	}
	if embedded == nil {
		path := fmt.Sprintf("/api/v3/movie/%d", owner)
		if c.kind == Sonarr {
			path = fmt.Sprintf("/api/v3/series/%d", owner)
		}
		embedded = &item{}
		if err := c.get(ctx, path, nil, embedded); err != nil {
			return nil, err
		}
	}
	m := c.media(*embedded)
	if !hasID(m) {
		return nil, nil
	}
	out := (&Match{Media: m, Episodes: episodeMap(m, episodes), DownloadID: strings.ToLower(hash), ItemID: owner}).withFacts(*embedded)
	if resolution > 0 {
		out.Resolution = strconv.Itoa(resolution) + "p"
	}
	return out, nil
}

// Parse matches a release name against titles already in the instance library.
func (c *Client) Parse(ctx context.Context, title string) (*Match, error) {
	var out struct {
		Series   *item     `json:"series"`
		Movie    *item     `json:"movie"`
		Episodes []episode `json:"episodes"`
	}
	if err := c.get(ctx, "/api/v3/parse", url.Values{"title": {title}}, &out); err != nil {
		return nil, err
	}
	v := out.Movie
	if c.kind == Sonarr {
		v = out.Series
	}
	if v == nil {
		return nil, nil
	}
	m := c.media(*v)
	if !hasID(m) {
		return nil, nil
	}
	return (&Match{Media: m, Episodes: episodeMap(m, out.Episodes), ItemID: v.ID}).withFacts(*v), nil
}

type importRecord struct {
	EventType  string   `json:"eventType"`
	DownloadID string   `json:"downloadId"`
	Episode    *episode `json:"episode"`
	Quality    struct {
		Quality struct {
			Resolution int `json:"resolution"`
		} `json:"quality"`
	} `json:"quality"`
	Data struct {
		DroppedPath string `json:"droppedPath"`
		Size        string `json:"size"`
	} `json:"data"`
}

// maxImportsResponse bounds the import history read for one title.
const maxImportsResponse = 32 << 20

// decodeArrayPrefix decodes a JSON array element by element. When the body
// exceeds limit it returns the elements read before the cut.
func decodeArrayPrefix[T any](r io.Reader, limit int64) ([]T, error) {
	counted := &countingReader{r: io.LimitReader(r, limit)}
	dec := json.NewDecoder(counted)
	if token, err := dec.Token(); err != nil || token != json.Delim('[') {
		return nil, errors.New("expected a JSON array")
	}
	out := []T{}
	for dec.More() {
		var v T
		if err := dec.Decode(&v); err != nil {
			if counted.n >= limit {
				return out, nil
			}
			return nil, err
		}
		out = append(out, v)
	}
	if _, err := dec.Token(); err != nil && counted.n < limit {
		return nil, err
	}
	return out, nil
}

type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// Imports lists the files imported for a library movie or series, newest first.
func (c *Client) Imports(ctx context.Context, itemID int) ([]ImportedFile, error) {
	if itemID <= 0 {
		return nil, nil
	}
	path, params := "/api/v3/history/movie", url.Values{"movieId": {strconv.Itoa(itemID)}, "eventType": {"3"}}
	if c.kind == Sonarr {
		path, params = "/api/v3/history/series", url.Values{"seriesId": {strconv.Itoa(itemID)}, "eventType": {"3"}, "includeEpisode": {"true"}}
	}
	var records []importRecord
	// A long series can hold more import records than one response allows;
	// records arrive newest first, so a cut keeps the newest imports.
	decode := func(body io.Reader) error {
		var err error
		records, err = decodeArrayPrefix[importRecord](body, maxImportsResponse)
		if err != nil {
			return fmt.Errorf("%s returned an invalid response", c.kind)
		}
		return nil
	}
	if err := c.get(ctx, path, params, decode); err != nil {
		return nil, err
	}
	out := []ImportedFile{}
	for _, r := range records {
		size, err := strconv.ParseInt(r.Data.Size, 10, 64)
		name := r.Data.DroppedPath[strings.LastIndexAny(r.Data.DroppedPath, `/\`)+1:]
		if r.EventType != "downloadFolderImported" || err != nil || size <= 0 || name == "" {
			continue
		}
		f := ImportedFile{Name: name, Size: size, DownloadID: strings.ToLower(r.DownloadID)}
		if r.Quality.Quality.Resolution > 0 {
			f.Resolution = strconv.Itoa(r.Quality.Quality.Resolution) + "p"
		}
		if e := r.Episode; e != nil && e.SeasonNumber > 0 && e.EpisodeNumber > 0 {
			f.Episode = &models.EpisodeMapping{Season: e.SeasonNumber, Episode: e.EpisodeNumber}
			if e.AbsoluteEpisodeNumber != nil {
				f.Episode.Absolute = *e.AbsoluteEpisodeNumber
			}
		}
		out = append(out, f)
	}
	return out, nil
}

// Lookup searches the instance's metadata provider by title. It accepts only
// one exact title match; movies also need a matching year.
func (c *Client) Lookup(ctx context.Context, title string, year int) (*Match, error) {
	if strings.TrimSpace(title) == "" || (c.kind == Radarr && year <= 0) {
		return nil, nil
	}
	path := "/api/v3/movie/lookup"
	if c.kind == Sonarr {
		path = "/api/v3/series/lookup"
	}
	var items []item
	if err := c.get(ctx, path, url.Values{"term": {title}}, &items); err != nil {
		return nil, err
	}
	want := releases.TitleKey(title)
	found := []models.Media{}
	for _, v := range items {
		names := []string{v.Title, v.OriginalTitle}
		for _, alt := range v.AlternateTitles {
			names = append(names, alt.Title)
		}
		if !slices.ContainsFunc(names, func(n string) bool { return n != "" && releases.TitleKey(n) == want }) {
			continue
		}
		if year > 0 && v.Year != year {
			continue
		}
		if m := c.media(v); hasID(m) && !slices.ContainsFunc(found, func(x models.Media) bool { return sameIDs(x, m) }) {
			found = append(found, m)
		}
	}
	if len(found) != 1 {
		return nil, nil
	}
	out := &Match{Media: found[0]}
	for _, v := range items {
		if sameIDs(c.media(v), found[0]) {
			out.ItemID = max(v.ID, 0)
			out.withFacts(v)
		}
	}
	return out, nil
}

func sameIDs(a, b models.Media) bool {
	return a.IMDbID == b.IMDbID && a.TMDbID == b.TMDbID && a.TVDbID == b.TVDbID
}

// LibraryFile is a file in the instance library with the video bitrate
// MediaInfo measured on import.
type LibraryFile struct {
	Size         int64
	VideoBitrate int64
}

// LibraryFiles lists the library files of a movie or series.
func (c *Client) LibraryFiles(ctx context.Context, itemID int) ([]LibraryFile, error) {
	if itemID <= 0 {
		return nil, nil
	}
	path, params := "/api/v3/moviefile", url.Values{"movieId": {strconv.Itoa(itemID)}}
	if c.kind == Sonarr {
		path, params = "/api/v3/episodefile", url.Values{"seriesId": {strconv.Itoa(itemID)}}
	}
	var files []struct {
		Size      int64 `json:"size"`
		MediaInfo *struct {
			VideoBitrate int64 `json:"videoBitrate"`
		} `json:"mediaInfo"`
	}
	if err := c.get(ctx, path, params, &files); err != nil {
		return nil, err
	}
	out := []LibraryFile{}
	for _, f := range files {
		if f.Size > 0 && f.MediaInfo != nil && f.MediaInfo.VideoBitrate > 0 {
			out = append(out, LibraryFile{Size: f.Size, VideoBitrate: f.MediaInfo.VideoBitrate})
		}
	}
	return out, nil
}
