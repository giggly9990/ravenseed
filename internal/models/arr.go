package models

import "time"

// ArrSettings lists the Sonarr and Radarr instances used to resolve media IDs
// for existing qui torrents.
type ArrSettings struct {
	Instances []ArrInstance `json:"instances"`
}

type ArrInstance struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	URL       string `json:"url"`
	APIKey    string `json:"apiKey,omitempty"`
	HasAPIKey bool   `json:"hasApiKey"`
	Enabled   bool   `json:"enabled"`
}

// Identity sources, strongest first. Only a download-history record is bound
// to the torrent hash itself; the others match the release name.
const (
	IdentityArrHistory = "arr_history"
	IdentityArrParse   = "arr_parse"
	IdentityArrLookup  = "arr_lookup"
	IdentityNone       = "none"
)

// TorrentIdentity is the media identity resolved for one torrent hash. It is
// cached by hash and reused by later monitor checks.
type TorrentIdentity struct {
	Hash     string `json:"hash"`
	Source   string `json:"source"`
	Instance string `json:"instance,omitempty"`
	// InstanceID identifies the Sonarr or Radarr instance; names may repeat.
	InstanceID string           `json:"instanceId,omitempty"`
	Media      Media            `json:"media"`
	Episodes   []EpisodeMapping `json:"episodes,omitempty"`
	Resolution string           `json:"resolution,omitempty"`
	// ViaHash is the cross-seeded copy whose download history matched.
	ViaHash string `json:"viaHash,omitempty"`
	// ImportedFile names the imported file matched by name and exact size.
	ImportedFile string `json:"importedFile,omitempty"`
	// Runtime is the movie or episode runtime in minutes; SeasonEpisodes
	// counts episodes per season. Encode bitrate tiers use both.
	Runtime        int         `json:"runtime,omitempty"`
	SeasonEpisodes map[int]int `json:"seasonEpisodes,omitempty"`
	// VideoMbps is the video bitrate MediaInfo measured on the library file
	// matching this torrent's video files by exact size; 0 when unknown.
	VideoMbps float64 `json:"videoMbps,omitempty"`
	// FactsVersion marks identities that carry runtime facts; older cached
	// entries resolve once more.
	FactsVersion int `json:"factsVersion,omitempty"`
	// ArrItemID is the library movie or series the identity points to.
	ArrItemID  int       `json:"arrItemId,omitempty"`
	ResolvedAt time.Time `json:"resolvedAt"`
	ExpiresAt  time.Time `json:"expiresAt"`
}

// Verified reports an identity bound to the torrent hash by download history.
func (v TorrentIdentity) Verified() bool { return v.Source == IdentityArrHistory }

// Found reports whether any source supplied media IDs.
func (v TorrentIdentity) Found() bool {
	return v.Source != IdentityNone && (v.Media.IMDbID != "" || v.Media.TMDbID > 0 || v.Media.TVDbID > 0)
}
