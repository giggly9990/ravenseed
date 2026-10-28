package policy

import "ravenseed/internal/models"

// Built-ins capture only observable slot and trump rules from the four supplied
// rulesets. Upload compliance and subjective claims remain outside this model.
func Presets() []models.PolicyPreset {
	first := Default(0)
	first.Enabled = true
	first.PresetID = "ruleset_1"
	first.EncodeSizePercent = 20
	// Two 1080p x264 slots, quality and retention; every other encode slot is single.
	first.EncodeSizeScope = []string{"1080p:AVC"}
	first.EncodeSizeSlots = 2
	first.EpisodesBlockedByPack = true
	// Internals may take an additional slot or trump; list the tracker's groups.
	first.InternalSlots = "review"
	first.ReviewDualAudio = true
	first.WebDLOverWebRip = true
	first.SeasonPackOverEpisodes = true
	first.HDRRelations = []models.HDRTrumpRelation{{From: "DV+HDR10", To: "HDR10"}, {From: "DV+HDR10+", To: "HDR10+"}}
	first.ReviewVisualQuality = true
	first.ReviewSuperiorMaster = true
	web := first.Slots["WEB_DL"]
	web.Provider = false
	first.Slots["WEB_DL"] = web
	first.Slots["WEBRIP"] = web

	second := Default(0)
	second.Enabled = true
	second.PresetID = "ruleset_2"
	second.WebDLOverWebRip = true
	second.Corrections = []string{"REPACK", "PROPER", "RERIP", "REISSUE"}
	second.SeasonPackOverEpisodes = true
	second.ReviewVisualQuality = true
	second.ReviewSuperiorMaster = true

	third := Default(0)
	third.Enabled = true
	third.PresetID = "ruleset_3"
	third.Corrections = []string{"REPACK", "PROPER"}
	third.SeasonPackOverEpisodes = true
	third.EpisodesBlockedByPack = true
	third.ReviewLosslessWeb = true
	third.HDRRelations = []models.HDRTrumpRelation{{From: "DV+HDR10", To: "HDR10"}}
	third.ReviewVisualQuality = true
	third.ReviewSuperiorMaster = true
	remux := third.Slots["REMUX"]
	remux.Hybrid = true
	third.Slots["REMUX"] = remux

	fourth := Default(0)
	fourth.Enabled = true
	fourth.PresetID = "ruleset_4"
	fourth.Corrections = []string{"REPACK", "PROPER", "RERIP", "REISSUE"}
	fourth.HDRRelations = []models.HDRTrumpRelation{
		{From: "DV+HDR10+", To: "DV+HDR10"},
		{From: "DV+HDR10+", To: "HDR10+"},
		{From: "DV+HDR10+", To: "HDR10"},
		{From: "DV+HDR10+", To: "PQ10"},
		{From: "DV+HDR10", To: "HDR10"},
		{From: "DV+HDR10", To: "PQ10"},
		{From: "HDR10+", To: "HDR10"},
		{From: "HDR10+", To: "PQ10"},
		{From: "HDR10", To: "PQ10"},
	}
	fourth.GroupOverNoGrp = true
	fourth.SeasonPackOverEpisodes = true
	fourth.EpisodesBlockedByPack = true
	// Internals coexist with any non-internal release; list the tracker's groups.
	fourth.InternalSlots = "separate"
	// Compact up to 12 Mbps video at 1080p and 18 Mbps at 2160p; transparent above.
	fourth.EncodeBitrateTiers = []models.EncodeBitrateTier{{Resolution: "1080p", MaxVideoMbps: 12}, {Resolution: "2160p", MaxVideoMbps: 18}}
	fourth.ReviewDualAudio = true
	fourth.ReviewVisualQuality = true
	fourth.ReviewSuperiorMaster = true
	fourth.ReviewHDRComparability = true

	return []models.PolicyPreset{
		{ID: "ruleset_1", Name: "Ruleset 1", Description: "Resolution slots, two 1080p x264 encode slots split at 20% size, correction, HDR and season-pack trumps; visual differences need review.", Policy: first},
		{ID: "ruleset_2", Name: "Ruleset 2", Description: "Provider-specific WEB slots, correction and season-pack trumps; HDR and master quality need review.", Policy: second},
		{ID: "ruleset_3", Name: "Ruleset 3", Description: "Provider WEB slots, separate hybrid remux slots, correction and season-pack trumps; the lossless 2160p WEB-DL slot and WEBRip quality need review.", Policy: third},
		{ID: "ruleset_4", Name: "Ruleset 4", Description: "Provider WEB, remux and encode slots, compact and transparent encode tiers by bitrate, configured HDR relations; reputable groups require an explicit list.", Policy: fourth},
	}
}
