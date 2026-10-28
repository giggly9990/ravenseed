package policy

import (
	"ravenseed/internal/models"
	"testing"
)

func web(id, provider, correction string) models.Result {
	// A correction is a different file, so it has different bytes.
	return models.Result{ID: id, Size: 1000 + int64(len(correction)), Parsed: models.ParsedRelease{MediaType: "movie", Title: "Example", Year: 2024, Structure: "movie", ReleaseType: "WEB_DL", Source: "WEB_DL", Service: provider, Resolution: "2160p", VideoCodec: "HEVC", HDR: []string{"HDR10"}, Correction: correction, Group: "GROUP"}}
}

func TestPolicySlotsAndObjectiveTrumps(t *testing.T) {
	v := Default(7)
	v.Enabled = true
	base := web("source", "AMZN", "")
	for _, test := range []struct {
		name      string
		candidate models.Result
		target    []models.Result
		want      models.CandidateClassification
	}{
		{"empty target", base, nil, models.PolicyMissingSlot},
		{"equivalent", base, []models.Result{web("existing", "AMZN", "")}, models.PolicyEquivalent},
		{"provider coexistence", base, []models.Result{web("existing", "NF", "")}, models.PolicyCoexistence},
		{"correction trump", web("source", "AMZN", "REPACK"), []models.Result{web("existing", "AMZN", "")}, models.PolicyTrump},
		{"reissue trump when enabled", web("source", "AMZN", "REISSUE"), []models.Result{web("existing", "AMZN", "")}, models.PolicyNotEligible},
		{"original cannot trump correction", base, []models.Result{web("existing", "AMZN", "REPACK")}, models.PolicyNotEligible},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := Evaluate(v, test.candidate, test.target, true)
			if got.Classification != test.want {
				t.Fatalf("got %s, want %s: %+v", got.Classification, test.want, got)
			}
		})
	}
	v.HDRRelations = []models.HDRTrumpRelation{{From: "DV+HDR10", To: "HDR10"}}
	dv := base
	dv.Parsed.HDR = []string{"DV", "HDR10"}
	if got := Evaluate(v, dv, []models.Result{web("existing", "AMZN", "")}, true); got.Classification != models.PolicyTrump {
		t.Fatalf("configured HDR relation did not trump: %+v", got)
	}
	v.Corrections = append(v.Corrections, "REISSUE")
	if got := Evaluate(v, web("source", "AMZN", "REISSUE"), []models.Result{web("existing", "AMZN", "")}, true); got.Classification != models.PolicyTrump {
		t.Fatalf("configured reissue did not trump: %+v", got)
	}
}

func TestPolicyReviewAndIncompleteSearch(t *testing.T) {
	v := Default(7)
	v.Enabled, v.ReviewVisualQuality = true, true
	base := web("source", "AMZN", "")
	other := web("other", "AMZN", "")
	other.Size = 1100
	other.Parsed.Group = "DIFFERENT"
	if got := Evaluate(v, base, []models.Result{other}, false); got.Classification != models.PolicyUnevaluated {
		t.Fatal("partial target search produced a conclusion", got)
	}
	if got := Evaluate(v, base, []models.Result{other}, true); got.Classification != models.PolicyPossibleTrump || len(got.ReviewReasons) == 0 {
		t.Fatalf("subjective rule did not require review: %+v", got)
	}
	base.Parsed.VideoCodec = ""
	if got := Evaluate(v, base, nil, true); got.Classification != models.PolicyUnevaluated || len(got.MissingEvidence) == 0 {
		t.Fatalf("missing slot metadata was treated as conclusive: %+v", got)
	}
}

func TestEncodeSizeThresholdDefinesSlotOnly(t *testing.T) {
	v := Default(7)
	v.Enabled = true
	v.EncodeSizePercent = 20
	a, b := web("a", "AMZN", ""), web("b", "AMZN", "")
	a.Parsed.ReleaseType, b.Parsed.ReleaseType = "ENCODE", "ENCODE"
	a.Parsed.Source, b.Parsed.Source = "BLURAY", "BLURAY"
	a.Size, b.Size = 1000, 900
	if got := Evaluate(v, a, []models.Result{b}, true); got.Classification != models.PolicyNotEligible {
		t.Fatalf("near-size encodes should compete without a quality claim: %+v", got)
	}
	b.Size = 700
	if got := Evaluate(v, a, []models.Result{b}, true); got.Classification != models.PolicyCoexistence {
		t.Fatalf("distant-size encodes should be separate slots: %+v", got)
	}
}

func TestSuppliedRulesetPresetsAreValidAndGeneric(t *testing.T) {
	presets := Presets()
	if len(presets) != 4 {
		t.Fatalf("got %d presets", len(presets))
	}
	for _, preset := range presets {
		preset.Policy.IndexerID = 7
		if err := Validate(preset.Policy); err != nil {
			t.Fatalf("%s: %v", preset.ID, err)
		}
		if !preset.Policy.Enabled || preset.Policy.PresetID != preset.ID {
			t.Fatalf("invalid preset identity: %+v", preset)
		}
	}
	if presets[0].Policy.EncodeSizePercent != 20 {
		t.Fatal("ruleset 1 lost its 20% encode size boundary")
	}
	if presets[2].Policy.WebDLOverWebRip {
		t.Fatal("ruleset 3 must not automatically trump a WEBRip")
	}
	fourth := presets[3].Policy
	fourth.IndexerID = 7
	dvPlus := web("candidate", "AMZN", "")
	dvPlus.Parsed.HDR = []string{"DV", "HDR10+"}
	for _, lower := range [][]string{{"DV", "HDR10"}, {"HDR10+"}, {"HDR10"}, {"PQ10"}} {
		existing := web("existing", "AMZN", "")
		existing.Parsed.HDR = lower
		if got := Evaluate(fourth, dvPlus, []models.Result{existing}, true); got.Classification != models.PolicyPossibleTrump || len(got.ReviewReasons) == 0 {
			t.Fatalf("ruleset 4 HDR ranking %v: %+v", lower, got)
		}
	}
	dvHDR := web("candidate", "AMZN", "")
	dvHDR.Parsed.HDR = []string{"DV", "HDR10"}
	hdrPlus := web("existing", "AMZN", "")
	hdrPlus.Parsed.HDR = []string{"HDR10+"}
	if got := Evaluate(fourth, dvHDR, []models.Result{hdrPlus}, true); got.Classification != models.PolicyCoexistence {
		t.Fatalf("ruleset 4 middle HDR tiers must coexist: %+v", got)
	}
}

func TestFullDiscDoesNotUseMediaIDAsDiscIdentity(t *testing.T) {
	v := Default(7)
	v.Enabled = true
	a, b := web("a", "", ""), web("b", "", "")
	a.Parsed.ReleaseType, b.Parsed.ReleaseType = "FULL_DISC", "FULL_DISC"
	a.Parsed.Source, b.Parsed.Source = "BLURAY", "BLURAY"
	a.Parsed.ContentID, b.Parsed.ContentID = "tmdb:123", "tmdb:123"
	b.Size = 900
	got := Evaluate(v, a, []models.Result{b}, true)
	if got.Classification != models.PolicyUnevaluated || len(got.MissingEvidence) == 0 {
		t.Fatalf("media ID was mistaken for disc pressing: %+v", got)
	}
}

func TestUnmarkedHDRCannotAutomaticallyTrump(t *testing.T) {
	v := Default(7)
	v.Enabled, v.DetectReview = true, true
	a, b := web("a", "AMZN", "REPACK"), web("b", "AMZN", "")
	a.Parsed.HDR, b.Parsed.HDR = nil, nil
	got := Evaluate(v, a, []models.Result{b}, true)
	if got.Classification != models.PolicyPossibleTrump || len(got.MissingEvidence) == 0 {
		t.Fatalf("unmarked HDR was treated as an automatic trump: %+v", got)
	}
}

func TestPolicyFindingCarriesTargetReleaseDetails(t *testing.T) {
	v := Default(7)
	v.Enabled = true
	candidate := web("candidate", "AMZN", "")
	target := web("5a82b2604597cff00e888009", "AMZN", "")
	target.Title = "Example.2024.2160p.AMZN.WEB-DL.HDR10-GROUP"
	target.Indexer = "Target Tracker"
	target.InfoURL = "https://tracker.example/torrents/123"
	got := Evaluate(v, candidate, []models.Result{target}, true)
	if len(got.TargetResults) != 1 || got.TargetResults[0].Title != target.Title || got.TargetResults[0].Indexer != target.Indexer || got.TargetResults[0].InfoURL != target.InfoURL || got.TargetResults[0].Relation != "equivalent" {
		t.Fatalf("target release details missing from finding: %+v", got)
	}
	target.Parsed.Service = "NF"
	got = Evaluate(v, candidate, []models.Result{target}, true)
	if got.Classification != models.PolicyCoexistence || len(got.TargetResults) != 1 || got.TargetResults[0].Relation != "different_slot" {
		t.Fatalf("coexisting target release details missing: %+v", got)
	}
}

func TestCrossSlotAndConfiguredObjectiveImprovements(t *testing.T) {
	v := Default(7)
	v.Enabled = true
	v.WebDLOverWebRip = true
	v.SeasonPackOverEpisodes = true
	v.GroupOverNoGrp = true
	v.ReputableGroups = []string{"TRUST"}
	v.SourcePriority = []string{"HDTV", "BLURAY"}
	base := web("candidate", "AMZN", "")
	old := web("old", "AMZN", "")
	old.Parsed.ReleaseType, old.Parsed.Source = "WEBRIP", "WEB"
	if got := Evaluate(v, base, []models.Result{old}, true); got.Classification != models.PolicyTrump {
		t.Fatalf("WEB-DL improvement: %+v", got)
	}

	base.Parsed.ReleaseType, old.Parsed.ReleaseType = "ENCODE", "ENCODE"
	base.Parsed.Source, old.Parsed.Source = "BLURAY", "HDTV"
	if got := Evaluate(v, base, []models.Result{old}, true); got.Classification != models.PolicyTrump {
		t.Fatalf("source priority: %+v", got)
	}

	base.Parsed.Source, old.Parsed.Source = "BLURAY", "BLURAY"
	base.Parsed.Group, old.Parsed.Group = "TRUST", ""
	if got := Evaluate(v, base, []models.Result{old}, true); got.Classification != models.PolicyTrump {
		t.Fatalf("reputable group: %+v", got)
	}

	base.Parsed.MediaType, old.Parsed.MediaType = "tv", "tv"
	base.Parsed.Season, old.Parsed.Season = 1, 1
	base.Parsed.Structure, old.Parsed.Structure = "seasons:1", "episodes:s1:e1"
	base.Parsed.Group, old.Parsed.Group = "GROUP", "GROUP"
	if got := Evaluate(v, base, []models.Result{old}, true); got.Classification != models.PolicyTrump {
		t.Fatalf("season pack: %+v", got)
	}
}

func TestOccupiedSlotIsNotSpeculativeTrump(t *testing.T) {
	v := Presets()[2].Policy
	a, b := web("source", "NF", ""), web("target", "NF", "")
	b.Size++
	got := Evaluate(v, a, []models.Result{b}, true)
	if got.Classification != models.PolicyNotEligible || len(got.ReviewReasons) != 0 {
		t.Fatalf("same release with differing size became a speculative trump: %+v", got)
	}
}

func TestExistingReleasePrecedesMissingPolicyMetadata(t *testing.T) {
	v := Default(7)
	v.Enabled = true
	a, b := web("source", "", ""), web("target", "", "")
	got := Evaluate(v, a, []models.Result{b}, true)
	if got.Classification != models.PolicyEquivalent {
		t.Fatalf("policy metadata hid existing release: %+v", got)
	}
}

func TestCorrectionOnlyTrumpsItsOwnGroup(t *testing.T) {
	v := Default(7)
	v.Enabled = true
	v.Corrections = append(v.Corrections, "REISSUE")
	other := web("existing", "AMZN", "")
	other.Parsed.Group = "CTRLHD"
	for _, kind := range []string{"REPACK", "RERIP", "REISSUE"} {
		got := Evaluate(v, web("source", "AMZN", kind), []models.Result{other}, true)
		if got.Classification != models.PolicyNotEligible || len(got.Improvements) != 0 {
			t.Fatalf("%s from another group counted as an improvement: %+v", kind, got)
		}
	}
	if got := Evaluate(v, web("source", "AMZN", "PROPER"), []models.Result{other}, true); got.Classification != models.PolicyTrump {
		t.Fatalf("PROPER fixing another group's release did not trump: %+v", got)
	}
	repacked := other
	repacked.Parsed.Correction = "REPACK"
	if got := Evaluate(v, web("source", "AMZN", ""), []models.Result{repacked}, true); len(got.Regressions) != 0 {
		t.Fatalf("another group's REPACK counted against an original: %+v", got)
	}
}
