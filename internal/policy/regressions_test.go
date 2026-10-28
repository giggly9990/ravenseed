package policy

import (
	"ravenseed/internal/models"
	"testing"
)

func TestMultiSeasonPackBlocksLaterSeasonEpisode(t *testing.T) {
	tv := models.Media{Type: "tv"}
	pack := parsed("pack", "Show.S01-S02.1080p.AMZN.WEB-DL.DDP5.1.H.264-GRP", 90000, tv)
	episode := parsed("ep", "Show.S02E01.1080p.AMZN.WEB-DL.DDP5.1.H.264-OTHER", 1000, tv)
	v := presetPolicy(t, "ruleset_4")
	if got := Evaluate(v, episode, []models.Result{pack}, true); got.Classification != models.PolicyNotEligible || len(got.CompetingResultIDs) != 1 {
		t.Fatalf("S02 episode offered beside an S01-S02 pack: %s %s", got.Classification, got.Reason)
	}
	third := parsed("ep3", "Show.S03E01.1080p.AMZN.WEB-DL.DDP5.1.H.264-OTHER", 1000, tv)
	if got := Evaluate(v, third, []models.Result{pack}, true); got.Classification != models.PolicyMissingSlot {
		t.Fatalf("pack blocked a season it does not contain: %s %s", got.Classification, got.Reason)
	}
}

func TestEpisodesNeedOverlapWithoutTVStructure(t *testing.T) {
	tv := models.Media{Type: "tv"}
	v := Default(7)
	v.Enabled = true
	web := v.Slots["WEB_DL"]
	web.TVStructure = false
	v.Slots["WEB_DL"] = web
	e3 := parsed("e3", "Show.S01E03.1080p.AMZN.WEB-DL.DDP5.1.H.264-AAA", 1000, tv)
	e4 := parsed("e4", "Show.S01E04.1080p.AMZN.WEB-DL.DDP5.1.H.264-BBB", 1100, tv)
	if got := Evaluate(v, e3, []models.Result{e4}, true); got.Classification != models.PolicyMissingSlot {
		t.Fatalf("another episode occupied the slot: %s %s", got.Classification, got.Reason)
	}
}

func TestSpecialsAndDailyEpisodesAreEvaluated(t *testing.T) {
	tv := models.Media{Type: "tv"}
	v := presetPolicy(t, "ruleset_4")
	special := parsed("s", "Show.S00E01.1080p.AMZN.WEB-DL.DDP5.1.H.264-AAA", 1000, tv)
	if got := Evaluate(v, special, nil, true); got.Classification != models.PolicyMissingSlot {
		t.Fatalf("S00 special not evaluated: %s %s", got.Classification, got.Reason)
	}
	day := parsed("d", "Show.2024.10.01.1080p.AMZN.WEB-DL.DDP5.1.H.264-AAA", 1000, tv)
	otherDay := parsed("o", "Show.2024.10.02.1080p.AMZN.WEB-DL.DDP5.1.H.264-BBB", 1000, tv)
	sameDay := parsed("x", "Show.2024.10.01.1080p.AMZN.WEB-DL.DDP5.1.H.264-BBB", 1100, tv)
	if got := Evaluate(v, day, []models.Result{otherDay}, true); got.Classification != models.PolicyMissingSlot {
		t.Fatalf("another air date occupied the slot: %s %s", got.Classification, got.Reason)
	}
	if got := Evaluate(v, day, []models.Result{sameDay}, true); got.Classification != models.PolicyPossibleTrump || len(got.CompetingResultIDs) != 1 {
		t.Fatalf("same air date did not compete: %s %s", got.Classification, got.Reason)
	}
	absolute := parsed("a", "[Group] Show - 05 (1080p)", 1000, tv)
	if got := Evaluate(v, absolute, nil, true); got.Classification != models.PolicyUnevaluated {
		t.Fatalf("absolute numbering evaluated: %s", got.Classification)
	}
}

func TestCrossSlotRulesKeepOtherSlotAxes(t *testing.T) {
	v := Default(7)
	v.Enabled = true
	v.GroupOverNoGrp = true
	v.ReputableGroups = []string{"TRUST"}
	trusted := web("candidate", "AMZN", "")
	trusted.Parsed.Group = "TRUST"
	nogrp := web("old", "NF", "")
	nogrp.Parsed.Group = ""
	if got := Evaluate(v, trusted, []models.Result{nogrp}, true); got.Classification != models.PolicyCoexistence || len(got.Improvements) != 0 {
		t.Fatalf("AMZN group trumped NF NoGrp: %s %+v", got.Classification, got.Improvements)
	}
	nogrp.Parsed.Service = "AMZN"
	if got := Evaluate(v, trusted, []models.Result{nogrp}, true); got.Classification != models.PolicyTrump {
		t.Fatalf("same-provider group did not trump NoGrp: %s %s", got.Classification, got.Reason)
	}

	v.SourcePriority = []string{"hdtv", "Blu-ray"}
	encode := web("candidate", "", "")
	encode.Parsed.ReleaseType, encode.Parsed.Source = "ENCODE", "BLURAY"
	hdtv := web("old", "", "")
	hdtv.Parsed.ReleaseType, hdtv.Parsed.Source = "ENCODE", "HDTV"
	if got := Evaluate(v, encode, []models.Result{hdtv}, true); got.Classification != models.PolicyTrump {
		t.Fatalf("source priority not read in canonical form: %s %s", got.Classification, got.Reason)
	}
	hdtv.Parsed.Cut = []string{"EXTENDED"}
	if got := Evaluate(v, encode, []models.Result{hdtv}, true); len(got.Improvements) != 0 {
		t.Fatalf("source priority traded away a cut: %+v", got.Improvements)
	}
}

func TestProperOverAnotherGroupsRepack(t *testing.T) {
	v := Default(7)
	v.Enabled = true
	repack := web("existing", "AMZN", "REPACK")
	repack.Parsed.Group = "CTRLHD"
	if got := Evaluate(v, web("source", "AMZN", "PROPER"), []models.Result{repack}, true); got.Classification != models.PolicyTrump {
		t.Fatalf("PROPER over another group's REPACK: %s %+v", got.Classification, got)
	}
	proper := web("existing", "AMZN", "PROPER")
	proper.Parsed.Group = "CTRLHD"
	if got := Evaluate(v, web("source", "AMZN", "REPACK"), []models.Result{proper}, true); got.Classification != models.PolicyNotEligible || len(got.Regressions) != 1 {
		t.Fatalf("REPACK against another group's PROPER: %s %+v", got.Classification, got.Regressions)
	}
}

func TestReviewPromotionNeedsEveryOccupantAndNoRegression(t *testing.T) {
	movie := models.Media{Type: "movie"}
	v := presetPolicy(t, "ruleset_4")
	v.ReviewVisualQuality, v.ReviewSuperiorMaster = false, false
	dual := parsed("d", "Movie.2013.1080p.AMZN.WEB-DL.Dual-Audio.DDP5.1.H.264-BBB", 3100, movie)
	single := parsed("e", "Movie.2013.1080p.AMZN.WEB-DL.DDP5.1.H.264-AAA", 3000, movie)
	otherDual := parsed("f", "Movie.2013.1080p.AMZN.WEB-DL.Dual-Audio.DDP5.1.H.264-CCC", 3200, movie)
	if got := Evaluate(v, dual, []models.Result{single, otherDual}, true); got.Classification != models.PolicyNotEligible {
		t.Fatalf("dual-audio review raised while a dual-audio occupant exists: %s %v", got.Classification, got.ReviewReasons)
	}
	repack := parsed("r", "Movie.2013.REPACK.1080p.AMZN.WEB-DL.DDP5.1.H.264-BBB", 3050, movie)
	if got := Evaluate(v, dual, []models.Result{repack}, true); got.Classification != models.PolicyNotEligible || len(got.Regressions) == 0 {
		t.Fatalf("review promotion ignored a regression: %s %+v", got.Classification, got.Regressions)
	}
}

func TestKnownTierOccupantKeepsNotEligible(t *testing.T) {
	movie := models.Media{Type: "movie"}
	v := presetPolicy(t, "ruleset_4")
	v.ReviewVisualQuality, v.ReviewSuperiorMaster = false, false
	ctx := Context{RuntimeMinutes: 100, SourceVideoMbps: 5}
	source := parsed("s", "Movie.2013.1080p.BluRay.DTS.x264-AAA", 5000000000, movie)
	// 6.5 Mbps total, 5 Mbps video: compact, like the source.
	compact := parsed("c", "Movie.2013.1080p.BluRay.DTS.x264-BBB", 4875000000, movie)
	// 13.5 Mbps total, 12 Mbps video: at the 12 Mbps limit, tier unknown.
	edge := parsed("e", "Movie.2013.1080p.BluRay.DTS.x264-CCC", 10125000000, movie)
	if got := EvaluateWith(v, source, []models.Result{edge}, true, ctx); got.Classification != models.PolicyPossibleTrump {
		t.Fatalf("unknown-tier occupant alone did not go to review: %s %s", got.Classification, got.Reason)
	}
	if got := EvaluateWith(v, source, []models.Result{compact, edge}, true, ctx); got.Classification != models.PolicyNotEligible {
		t.Fatalf("known-tier occupant's verdict overwritten: %s %s", got.Classification, got.Reason)
	}
}

func TestValidateCanonicalForms(t *testing.T) {
	v := presetPolicy(t, "ruleset_4")
	for name, change := range map[string]func(*models.IndexerPolicy){
		"empty banned group":   func(p *models.IndexerPolicy) { p.BannedGroups = []string{" "} },
		"empty internal group": func(p *models.IndexerPolicy) { p.InternalGroups = []string{""} },
		"same HDR spelled twice": func(p *models.IndexerPolicy) {
			p.HDRRelations = []models.HDRTrumpRelation{{From: "HDR10+DV", To: "DV+HDR10"}}
		},
		"source twice": func(p *models.IndexerPolicy) { p.SourcePriority = []string{"BLURAY", "blu-ray"} },
	} {
		bad := v
		change(&bad)
		if Validate(bad) == nil {
			t.Errorf("%s accepted", name)
		}
	}
	relation := Default(7)
	relation.Enabled = true
	relation.HDRRelations = []models.HDRTrumpRelation{{From: "hdr10+dovi", To: "hdr"}}
	dv := web("source", "AMZN", "")
	dv.Parsed.HDR = []string{"DV", "HDR10"}
	if got := Evaluate(relation, dv, []models.Result{web("existing", "AMZN", "")}, true); got.Classification != models.PolicyTrump {
		t.Fatalf("HDR relation not read in canonical form: %s %s", got.Classification, got.Reason)
	}
	scoped := parsed("a", "Movie.2013.1080p.BluRay.x264-AAA", 1000, models.Media{Type: "movie"})
	if !inSizeScope(models.IndexerPolicy{EncodeSizeScope: []string{"1080p:x264"}}, scoped.Parsed) {
		t.Fatal("x264 scope did not cover an AVC encode")
	}
}

func TestBannedIgnoresExtensionCaseAndEmptyNames(t *testing.T) {
	v := models.IndexerPolicy{BannedGroups: []string{"YTS.MX"}}
	for _, title := range []string{"Movie.2013.1080p.BluRay.x264-YTS.MX.MKV", "Movie.2013.1080p.BluRay.x264-YTS.MX.torrent"} {
		if !Banned(v, models.Result{Title: title}) {
			t.Errorf("%s not banned", title)
		}
	}
	empty := models.IndexerPolicy{BannedGroups: []string{""}}
	if Banned(empty, models.Result{Title: "Movie.2013.1080p.BluRay.x264-"}) {
		t.Fatal("empty banned name matched")
	}
}

func TestUnknownResolutionIsNotLikelySDR(t *testing.T) {
	if likelySDR(models.ParsedRelease{VideoCodec: "AVC"}) {
		t.Fatal("unknown resolution assumed SDR")
	}
}

func TestCanonicalHDR(t *testing.T) {
	for in, want := range map[string]string{"HDR10+DV": "DV+HDR10", "DV+HDR10+": "DV+HDR10+", "HDR10++DV": "DV+HDR10+", "hdr10plus": "HDR10+", "DoVi HDR": "DV+HDR10", "PQ10": "PQ10"} {
		if got := canonicalHDR(in); got != want {
			t.Errorf("%s: %s, want %s", in, got, want)
		}
	}
}
