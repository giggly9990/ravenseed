package models

import "time"

type CreateMonitorRequest struct {
	SourceIndexerID   int    `json:"sourceIndexerId"`
	SourceTrackerHost string `json:"sourceTrackerHost"`
	TargetIndexerID   int    `json:"targetIndexerId"`
	IntervalHours     int    `json:"intervalHours"`
	InstanceIDs       []int  `json:"instanceIds"`
}

// MonitorStateRequest changes a monitor; at least one field is required.
type MonitorStateRequest struct {
	Enabled *bool `json:"enabled,omitempty"`
	// InstanceIDs replaces the qui instances the monitor scans; an empty
	// list scans every instance.
	InstanceIDs *[]int `json:"instanceIds,omitempty"`
}

type MonitorUploadRequest struct {
	InstanceID     int     `json:"instanceId"`
	TMDbID         int     `json:"tmdbId,omitempty"`
	UploadPresetID *string `json:"uploadPresetId,omitempty"`
	// ClearMediaIDs sends no resolved IDs, so Upload-Assistant identifies the
	// release itself. A TMDb ID in the same request still applies.
	ClearMediaIDs bool `json:"clearMediaIds,omitempty"`
	// ConfirmMediaIDs sends IDs from a Sonarr/Radarr library name match.
	// Without it only download-history IDs are sent.
	ConfirmMediaIDs bool `json:"confirmMediaIds,omitempty"`
}

type Monitor struct {
	ID                string     `json:"id"`
	SourceIndexerID   int        `json:"sourceIndexerId"`
	SourceIndexerName string     `json:"sourceIndexerName"`
	SourceTrackerHost string     `json:"sourceTrackerHost"`
	TargetIndexerID   int        `json:"targetIndexerId"`
	TargetIndexerName string     `json:"targetIndexerName"`
	IntervalHours     int        `json:"intervalHours"`
	Enabled           bool       `json:"enabled"`
	QuiURL            string     `json:"quiUrl"`
	CreatedAt         time.Time  `json:"createdAt"`
	LastScanAt        *time.Time `json:"lastScanAt,omitempty"`
	LastError         string     `json:"lastError,omitempty"`
	// InstanceIDs limits the scan to these qui instances. Monitors saved
	// before instance selection have none and scan every instance.
	InstanceIDs []int `json:"instanceIds,omitempty"`
}

type MonitorCandidate struct {
	TargetQuery       string            `json:"targetQuery,omitempty"`
	TargetResultCount int               `json:"targetResultCount"`
	ScopedResultCount int               `json:"scopedResultCount"`
	Policy            *PolicyEvaluation `json:"policy,omitempty"`
	Parsed            ParsedRelease     `json:"parsed"`
	ID                string            `json:"id"`
	MonitorID         string            `json:"monitorId"`
	InstanceID        int               `json:"instanceId"`
	Hash              string            `json:"hash"`
	Name              string            `json:"name"`
	Size              int64             `json:"size"`
	Status            string            `json:"status"`
	Message           string            `json:"message"`
	Media             Media             `json:"media"`
	Category          string            `json:"category,omitempty"`
	Identity          *TorrentIdentity  `json:"identity,omitempty"`
	IdentityError     string            `json:"identityError,omitempty"`
	NextCheckAt       time.Time         `json:"nextCheckAt"`
	LastCheckedAt     *time.Time        `json:"lastCheckedAt,omitempty"`
	MatchCount        int               `json:"matchCount"`
	UploadJobID       string            `json:"uploadJobId,omitempty"`
	// Upgrade is "upgrade" or "possible" when the target holds this torrent's
	// slot with a release that may replace it in the library.
	Upgrade          string              `json:"upgrade,omitempty"`
	UpgradeOffer     *PolicyTargetResult `json:"upgradeOffer,omitempty"`
	UpgradeDismissed string              `json:"upgradeDismissed,omitempty"`
}

type TrackerHost struct {
	Host  string `json:"host"`
	Count int    `json:"count"`
	// InstanceCounts is the torrent count per qui instance ID.
	InstanceCounts map[int]int `json:"instanceCounts"`
}
