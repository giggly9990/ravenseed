package models

import (
	"encoding/json"
	"time"
)

type IndexerSearchOverride struct {
	Movie string `json:"movie"`
	TV    string `json:"tv"`
}

type Indexer struct {
	SearchOverride *IndexerSearchOverride `json:"searchOverride,omitempty"`
	ID             int                    `json:"id"`
	Name           string                 `json:"name"`
	Enabled        bool                   `json:"enabled"`
	Backend        string                 `json:"backend"`
	Capabilities   []string               `json:"capabilities"`
}

type Media struct {
	Type          string `json:"type"`
	IMDbID        string `json:"imdbId,omitempty"`
	TMDbID        int    `json:"tmdbId,omitempty"`
	TVDbID        int    `json:"tvdbId,omitempty"`
	Title         string `json:"title,omitempty"`
	OriginalTitle string `json:"originalTitle,omitempty"`
	Year          int    `json:"year,omitempty"`
	Season        *int   `json:"season,omitempty"`
	Episode       *int   `json:"episode,omitempty"`
}

type SearchRequest struct {
	Media           Media `json:"media"`
	IndexerIDs      []int `json:"indexerIds"`
	TargetIndexerID int   `json:"targetIndexerId"`
	Filters         struct {
		MinimumCoverage int `json:"minimumCoverage"`
	} `json:"filters"`
	Refresh bool `json:"refresh,omitempty"`
}

type ParsedRelease struct {
	Checksum         string   `json:"checksum,omitempty"`
	ContentID        string   `json:"contentId,omitempty"`
	Seasons          []int    `json:"seasons,omitempty"`
	Episodes         []int    `json:"episodes,omitempty"`
	AbsoluteEpisodes []int    `json:"absoluteEpisodes,omitempty"`
	Correction       string   `json:"correction,omitempty"`
	Revision         int      `json:"revision,omitempty"`
	MediaType        string   `json:"mediaType"`
	Title            string   `json:"title"`
	Year             int      `json:"year"`
	Season           int      `json:"season"`
	Episode          int      `json:"episode"`
	Structure        string   `json:"structure"`
	Resolution       string   `json:"resolution"`
	ReleaseType      string   `json:"releaseType"`
	Source           string   `json:"source"`
	VideoCodec       string   `json:"videoCodec"`
	BitDepth         string   `json:"bitDepth"`
	HDR              []string `json:"hdr"`
	AudioCodec       string   `json:"audioCodec"`
	AudioChannels    string   `json:"audioChannels"`
	Edition          []string `json:"edition"`
	Cut              []string `json:"cut"`
	Group            string   `json:"group"`
	DisplayGroup     string   `json:"displayGroup"`
	Site             string   `json:"site"`
	Repack           bool     `json:"repack"`
	Proper           bool     `json:"proper"`
	Hybrid           bool     `json:"hybrid"`
	Service          string   `json:"service"`
	Language         []string `json:"language"`
}

type Result struct {
	ID           string `json:"id"`
	IndexerID    int    `json:"indexerId"`
	Indexer      string `json:"indexer"`
	Title        string `json:"title"`
	Size         int64  `json:"size"`
	Seeders      int    `json:"seeders"`
	Leechers     int    `json:"leechers"`
	CategoryID   int    `json:"categoryId"`
	CategoryName string `json:"categoryName"`
	// DownloadURL often carries a tracker passkey. It is stored, but never
	// sent to the browser: Result.MarshalJSON reports HasDownloadURL instead.
	DownloadURL           string          `json:"downloadUrl,omitempty" openapi:"-"`
	HasDownloadURL        bool            `json:"hasDownloadUrl"`
	InfoURL               string          `json:"infoUrl"`
	PublishDate           time.Time       `json:"publishDate"`
	DownloadFactor        *float64        `json:"downloadFactor,omitempty"`
	UploadFactor          *float64        `json:"uploadFactor,omitempty"`
	Raw                   json.RawMessage `json:"raw"`
	ParserOutput          json.RawMessage `json:"parserOutput"`
	Parsed                ParsedRelease   `json:"parsed"`
	IdentityEvidence      []string        `json:"identityEvidence,omitempty"`
	IdentityVerified      bool            `json:"identityVerified"`
	Confidence            string          `json:"confidence"`
	Evidence              []string        `json:"evidence"`
	NormalizationEvidence []string        `json:"normalizationEvidence,omitempty"`
}

type SizeVariant struct {
	Policy       *PolicyEvaluation `json:"policy,omitempty"`
	TargetStatus *TargetStatus     `json:"targetStatus,omitempty"`
	Size         int64             `json:"size"`
	ResultIDs    []string          `json:"resultIds"`
	IndexerIDs   []int             `json:"indexerIds"`
	Coverage     int               `json:"coverage"`
	Confidence   string            `json:"confidence"`
}

type TargetStatus struct {
	IndexerID         int    `json:"indexerId"`
	Evaluated         bool   `json:"evaluated"`
	ExactMatchFound   bool   `json:"exactMatchFound"`
	RelatedMatchFound bool   `json:"relatedMatchFound"`
	Candidate         bool   `json:"candidate"`
	Message           string `json:"message"`
}

type Family struct {
	Policy          *PolicyEvaluation `json:"policy,omitempty"`
	RelatedFamilies []FamilyRelation  `json:"relatedFamilies,omitempty"`
	ID              string            `json:"id"`
	Canonical       ParsedRelease     `json:"canonical"`
	Results         []Result          `json:"results"`
	Variants        []SizeVariant     `json:"variants"`
	Coverage        int               `json:"coverage"`
	TotalCoverage   int               `json:"totalCoverage"`
	Confidence      string            `json:"confidence"`
	Evidence        []string          `json:"evidence"`
	TargetStatus    TargetStatus      `json:"targetStatus"`
	Seeders         int               `json:"seeders"`
	Completeness    int               `json:"completeness"`
	Newest          time.Time         `json:"newest"`
}

type IndexerProgress struct {
	Strategy    *SearchStrategy `json:"strategy,omitempty"`
	IndexerID   int             `json:"indexerId"`
	Name        string          `json:"name"`
	Status      string          `json:"status"`
	ResultCount int             `json:"resultCount"`
	Error       string          `json:"error,omitempty"`
}

type IndexerError struct {
	IndexerID int    `json:"indexerId"`
	Error     string `json:"error"`
}

type Job struct {
	PolicySnapshotID string            `json:"policySnapshotId,omitempty"`
	Media            *MediaSnapshot    `json:"media,omitempty"`
	ID               string            `json:"id"`
	Status           string            `json:"status"`
	Request          SearchRequest     `json:"request"`
	StartedAt        time.Time         `json:"startedAt"`
	CompletedAt      *time.Time        `json:"completedAt,omitempty"`
	Indexers         []IndexerProgress `json:"indexers"`
	IndexerErrors    []IndexerError    `json:"indexerErrors"`
	ResultCount      int               `json:"resultCount"`
	FamilyCount      int               `json:"familyCount"`
	CandidateCount   int               `json:"candidateCount"`
	Error            string            `json:"error,omitempty"`
	CacheHit         bool              `json:"cacheHit"`
	MatchingRules    *MatchingRules    `json:"matchingRules,omitempty"`
}

type Settings struct {
	SearchOverrides      map[int]IndexerSearchOverride `json:"searchOverrides,omitempty"`
	Metadata             *MetadataSettings             `json:"metadata,omitempty"`
	QuiURL               string                        `json:"quiUrl"`
	APIKey               string                        `json:"apiKey,omitempty"`
	HasAPIKey            bool                          `json:"hasApiKey"`
	DefaultTargetIndexer int                           `json:"defaultTargetIndexer"`
	DefaultMinCoverage   int                           `json:"defaultMinCoverage"`
	EnvironmentManaged   bool                          `json:"environmentManaged"`
	Matching             *MatchingRules                `json:"matching,omitempty"`
}

type MetadataSettings struct {
	Token              string `json:"token,omitempty"`
	HasToken           bool   `json:"hasToken"`
	EnvironmentManaged bool   `json:"environmentManaged"`
	Language           string `json:"language"`
	ClearToken         bool   `json:"clearToken,omitempty"`
}

type ResolveRequest struct {
	ID      string `json:"id"`
	Type    string `json:"type,omitempty"`
	Season  *int   `json:"season,omitempty"`
	Refresh bool   `json:"refresh,omitempty"`
}

type SeasonMetadata struct {
	Number       int             `json:"number"`
	Name         string          `json:"name"`
	AirDate      string          `json:"airDate"`
	EpisodeCount int             `json:"episodeCount"`
	Overview     string          `json:"overview"`
	PosterPath   string          `json:"posterPath"`
	Raw          json.RawMessage `json:"-"`
}

type MediaSnapshot struct {
	Media
	Overview      string           `json:"overview,omitempty"`
	PosterPath    string           `json:"posterPath,omitempty"`
	BackdropPath  string           `json:"backdropPath,omitempty"`
	Seasons       []SeasonMetadata `json:"seasons,omitempty"`
	SeasonDetails *SeasonMetadata  `json:"seasonDetails,omitempty"`
	// Runtime is the movie or typical episode runtime in minutes.
	Runtime   int       `json:"runtime,omitempty"`
	FetchedAt time.Time `json:"fetchedAt"`
	ExpiresAt time.Time `json:"expiresAt"`
	Cached    bool      `json:"cached"`
	Warning   string    `json:"warning,omitempty"`
}

type SearchStrategy struct {
	OmitTVParameters bool   `json:"omitTVParameters,omitempty"`
	Reason           string `json:"reason,omitempty"`
	Method           string `json:"method"`
	Query            string `json:"query"`
}

type FamilyRelation struct {
	FamilyID   string   `json:"familyId"`
	Confidence string   `json:"confidence"`
	Evidence   []string `json:"evidence"`
}

type MatchingRules struct {
	MatchAlternateTitles bool              `json:"matchAlternateTitles"`
	SizeTolerancePercent float64           `json:"sizeTolerancePercent"`
	GroupAliases         map[string]string `json:"groupAliases"`
	IgnoredTags          []string          `json:"ignoredTags"`
	EpisodeMappings      []EpisodeMapping  `json:"episodeMappings"`
}

type EpisodeMapping struct {
	IMDbID   string `json:"imdbId,omitempty"`
	TMDbID   int    `json:"tmdbId,omitempty"`
	Absolute int    `json:"absolute"`
	Season   int    `json:"season"`
	Episode  int    `json:"episode"`
}

type IndexerPreset struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	IndexerIDs      []int  `json:"indexerIds"`
	TargetIndexerID int    `json:"targetIndexerId"`
}

type QuiInstance struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Connected bool   `json:"connected"`
	IsActive  bool   `json:"isActive"`
}

type AddTorrentRequest struct {
	UploadPresetID      *string `json:"uploadPresetId,omitempty"`
	UploadAfterDownload bool    `json:"uploadAfterDownload,omitempty"`
	InstanceID          int     `json:"instanceId"`
	Category            string  `json:"category,omitempty"`
	SavePath            string  `json:"savePath,omitempty"`
	Tags                string  `json:"tags,omitempty"`
	Paused              *bool   `json:"paused,omitempty"`
}

type AddTorrentResponse struct {
	UploadJobID string `json:"uploadJobId,omitempty"`
	Added       int    `json:"added"`
	Failed      int    `json:"failed"`
}
