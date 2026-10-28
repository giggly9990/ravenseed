package models

import (
	"encoding/json"
	"time"
)

type CandidateClassification string

const (
	PolicyEquivalent    CandidateClassification = "equivalent"
	PolicyMissingSlot   CandidateClassification = "missing_slot"
	PolicyCoexistence   CandidateClassification = "coexistence"
	PolicyTrump         CandidateClassification = "trump"
	PolicyPossibleTrump CandidateClassification = "possible_trump"
	PolicyNotEligible   CandidateClassification = "not_eligible"
	PolicyUnevaluated   CandidateClassification = "unevaluated"
)

// SlotAxes select the metadata that must agree for two releases to compete.
// Unmarked HDR and cut are kept distinct from explicitly labeled values.
type SlotAxes struct {
	Resolution   bool `json:"resolution"`
	ReleaseClass bool `json:"releaseClass"`
	Source       bool `json:"source"`
	Provider     bool `json:"provider"`
	Codec        bool `json:"codec"`
	HDR          bool `json:"hdr"`
	Cut          bool `json:"cut"`
	TVStructure  bool `json:"tvStructure"`
	Group        bool `json:"group"`
	Hybrid       bool `json:"hybrid"`
}

type HDRTrumpRelation struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// EncodeBitrateTier splits encodes at one resolution into a compact tier, up
// to MaxVideoMbps of video, and a transparent tier above it.
type EncodeBitrateTier struct {
	Resolution   string  `json:"resolution"`
	MaxVideoMbps float64 `json:"maxVideoMbps"`
}

type IndexerPolicy struct {
	IndexerID         int                 `json:"indexerId"`
	Enabled           bool                `json:"enabled"`
	DetectMissing     bool                `json:"detectMissing"`
	DetectCoexistence bool                `json:"detectCoexistence"`
	DetectTrump       bool                `json:"detectTrump"`
	DetectReview      bool                `json:"detectReview"`
	Slots             map[string]SlotAxes `json:"slots"`
	EncodeSizePercent float64             `json:"encodeSizePercent"`
	// EncodeSizeScope limits the size boundary to "resolution:codec" pairs
	// such as "1080p:AVC"; empty applies it to every encode.
	EncodeSizeScope []string `json:"encodeSizeScope"`
	// EncodeSizeSlots caps the size-separated slots; 0 leaves them unlimited.
	EncodeSizeSlots    int                 `json:"encodeSizeSlots"`
	EncodeBitrateTiers []EncodeBitrateTier `json:"encodeBitrateTiers"`
	// EpisodesBlockedByPack refuses single episodes once any pack of their
	// season is on the target, whatever its quality or provider.
	EpisodesBlockedByPack  bool               `json:"episodesBlockedByPack"`
	Corrections            []string           `json:"corrections"`
	WebDLOverWebRip        bool               `json:"webDlOverWebRip"`
	SourcePriority         []string           `json:"sourcePriority"`
	HDRRelations           []HDRTrumpRelation `json:"hdrRelations"`
	GroupOverNoGrp         bool               `json:"groupOverNoGrp"`
	ReputableGroups        []string           `json:"reputableGroups"`
	SeasonPackOverEpisodes bool               `json:"seasonPackOverEpisodes"`
	DeadTorrentMaxSeeders  *int               `json:"deadTorrentMaxSeeders,omitempty"`
	ReviewVisualQuality    bool               `json:"reviewVisualQuality"`
	ReviewSuperiorMaster   bool               `json:"reviewSuperiorMaster"`
	ReviewHDRComparability bool               `json:"reviewHDRComparability"`
	// Rules the release name cannot settle; an occupied slot goes to review.
	ReviewDualAudio   bool `json:"reviewDualAudio"`
	ReviewLosslessWeb bool `json:"reviewLosslessWeb"`
	// BannedGroups are refused by this target tracker: their releases are not
	// offered, and an occupant from one can be trumped.
	BannedGroups []string `json:"bannedGroups"`
	// InternalGroups are this target tracker's own groups. Internal status
	// exists only on its home tracker, so the list belongs to one policy.
	InternalGroups []string `json:"internalGroups"`
	// InternalSlots: "" ignores internals, "separate" gives internals their
	// own slot beside non-internal releases, "review" sends an internal that
	// meets an occupied non-internal slot to review (extra slot or trump).
	InternalSlots string    `json:"internalSlots,omitempty"`
	PresetID      string    `json:"presetId,omitempty"`
	SnapshotID    string    `json:"snapshotId,omitempty"`
	UpdatedAt     time.Time `json:"updatedAt,omitzero"`
}

// MarshalJSON writes empty lists, not null, for policies saved before a list
// setting existed.
func (v IndexerPolicy) MarshalJSON() ([]byte, error) {
	type plain IndexerPolicy
	if v.EncodeSizeScope == nil {
		v.EncodeSizeScope = []string{}
	}
	if v.EncodeBitrateTiers == nil {
		v.EncodeBitrateTiers = []EncodeBitrateTier{}
	}
	if v.InternalGroups == nil {
		v.InternalGroups = []string{}
	}
	if v.BannedGroups == nil {
		v.BannedGroups = []string{}
	}
	return json.Marshal(plain(v))
}

type PolicyPreset struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Policy      IndexerPolicy `json:"policy"`
}

type PolicyEvidence struct {
	Rule       string `json:"rule"`
	Detail     string `json:"detail"`
	Capability string `json:"capability"`
}

type PolicyImprovement struct {
	Type string `json:"type"`
	From string `json:"from"`
	To   string `json:"to"`
}

// MarshalJSON keeps a credential-bearing info link out of stored and sent
// evaluations, such as an upgrade offer.
func (r PolicyTargetResult) MarshalJSON() ([]byte, error) {
	type plain PolicyTargetResult
	p := plain(r)
	p.InfoURL = PublicInfoURL(r.InfoURL, "")
	return json.Marshal(p)
}

type PolicyTargetResult struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Indexer  string `json:"indexer"`
	Size     int64  `json:"size"`
	InfoURL  string `json:"infoUrl,omitempty"`
	Relation string `json:"relation"`
	// Upgrade lists configured rules by which this target release beats the
	// candidate, from the candidate's value to the target's.
	Upgrade []PolicyImprovement `json:"upgrade,omitempty"`
	// UpgradeReview explains why the upgrade still needs a manual comparison.
	UpgradeReview string `json:"upgradeReview,omitempty"`
	// Group is the target release's group, for comparing occupants.
	Group string `json:"group,omitempty"`
}

type PolicyEvaluation struct {
	Classification     CandidateClassification `json:"classification"`
	SlotKey            string                  `json:"slotKey,omitempty"`
	SlotComponents     map[string]string       `json:"slotComponents,omitempty"`
	AutomaticEvidence  []PolicyEvidence        `json:"automaticEvidence"`
	MissingEvidence    []PolicyEvidence        `json:"missingEvidence"`
	ReviewReasons      []string                `json:"reviewReasons"`
	CompetingResultIDs []string                `json:"competingResultIds"`
	TargetResults      []PolicyTargetResult    `json:"targetResults"`
	Improvements       []PolicyImprovement     `json:"improvements"`
	Regressions        []PolicyImprovement     `json:"regressions"`
	Reason             string                  `json:"reason"`
	PolicySnapshotID   string                  `json:"policySnapshotId,omitempty"`
	EvaluatedAt        time.Time               `json:"evaluatedAt"`
}
