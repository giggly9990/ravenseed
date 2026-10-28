package models

import "time"

type UploadSettings struct {
	FlagPresets         []UploadFlagPreset `json:"flagPresets,omitempty"`
	DefaultFlagPresetID string             `json:"defaultFlagPresetId,omitempty"`
	Enabled             bool               `json:"enabled"`
	URL                 string             `json:"url"`
	Token               string             `json:"token,omitempty"`
	HasToken            bool               `json:"hasToken"`
	ClearToken          bool               `json:"clearToken,omitempty"`
	Trackers            []UploadTracker    `json:"trackers"`
	Paths               []UploadPath       `json:"paths"`
}

type UploadFlagPreset struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Args string `json:"args"`
}

type UploadTracker struct {
	IndexerID int    `json:"indexerId"`
	Code      string `json:"code"`
}

type UploadPath struct {
	InstanceID int    `json:"instanceId"`
	From       string `json:"from"`
	To         string `json:"to"`
}

// UploadStatuses lists every upload job status: active (adding, downloading,
// uploading), needing the user (needs_review, failed) and done (finished,
// cancelled).
var UploadStatuses = []string{"adding", "downloading", "uploading", "needs_review", "failed", "finished", "cancelled"}

type UploadJob struct {
	Attempt          int             `json:"attempt"`
	RateLimitRetries int             `json:"rateLimitRetries,omitempty"`
	SubmittedAt      *time.Time      `json:"submittedAt,omitempty"`
	NextAttemptAt    *time.Time      `json:"nextAttemptAt,omitempty"`
	PreviousAttempts []UploadAttempt `json:"previousAttempts,omitempty"`
	FlagPresetName   string          `json:"flagPresetName,omitempty"`
	Args             string          `json:"args,omitempty"`
	ID               string          `json:"id"`
	SearchID         string          `json:"searchId"`
	ResultID         string          `json:"resultId"`
	Title            string          `json:"title"`
	InstanceID       int             `json:"instanceId"`
	TargetIndexerID  int             `json:"targetIndexerId"`
	Tracker          string          `json:"tracker"`
	Tag              string          `json:"tag"`
	Hash             string          `json:"hash,omitempty"`
	Status           string          `json:"status"`
	Message          string          `json:"message"`
	Progress         float64         `json:"progress"`
	ContentPath      string          `json:"contentPath,omitempty"`
	UploadPath       string          `json:"uploadPath,omitempty"`
	Media            Media           `json:"media"`
	QuiURL           string          `json:"quiUrl"`
	AssistantURL     string          `json:"assistantUrl"`
	Paths            []UploadPath    `json:"paths"`
	TrackerResult    *TrackerResult  `json:"trackerResult,omitempty"`
	CreatedAt        time.Time       `json:"createdAt"`
	UpdatedAt        time.Time       `json:"updatedAt"`
}

// TrackerResult is the target tracker's outcome as Upload-Assistant reported
// it in its structured run events. UA redacts the text before sending it.
type TrackerResult struct {
	// Status is success, failed, skipped, dupe or banned.
	Status    string `json:"status"`
	Message   string `json:"message,omitempty"`
	Link      string `json:"link,omitempty"`
	TorrentID string `json:"torrentId,omitempty"`
}

type UploadAttempt struct {
	Attempt       int            `json:"attempt"`
	Args          string         `json:"args,omitempty"`
	Media         Media          `json:"media"`
	Status        string         `json:"status"`
	Message       string         `json:"message"`
	TrackerResult *TrackerResult `json:"trackerResult,omitempty"`
	UpdatedAt     time.Time      `json:"updatedAt"`
}

type RetryUploadRequest struct {
	Attempt int    `json:"attempt"`
	Args    string `json:"args"`
	TMDbID  *int   `json:"tmdbId,omitempty"`
}
