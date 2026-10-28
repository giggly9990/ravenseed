package policy

import (
	"ravenseed/internal/models"
	"ravenseed/internal/releases"
	"testing"
)

func parsed(id, title string, size int64, media models.Media) models.Result {
	v := models.Result{ID: id, Title: title, Size: size}
	releases.Parse(&v, media)
	return v
}

func presetPolicy(t *testing.T, id string) models.IndexerPolicy {
	t.Helper()
	for _, p := range Presets() {
		if p.ID == id {
			p.Policy.IndexerID = 2
			return p.Policy
		}
	}
	t.Fatalf("no preset %s", id)
	return models.IndexerPolicy{}
}

func TestSeasonPackRulesUseParsedStructures(t *testing.T) {
	tv := models.Media{Type: "tv"}
	pack := parsed("pack", "Show.S01.1080p.AMZN.WEB-DL.DDP5.1.H.264-GRP", 9000, tv)
	episode := parsed("ep", "Show.S01E01.1080p.AMZN.WEB-DL.DDP5.1.H.264-OTHER", 1000, tv)
	for _, id := range []string{"ruleset_1", "ruleset_2", "ruleset_3", "ruleset_4"} {
		v := presetPolicy(t, id)
		if got := Evaluate(v, episode, []models.Result{pack}, true); got.Classification != models.PolicyNotEligible {
			t.Errorf("%s: single episode offered while the season pack exists: %s %s", id, got.Classification, got.Reason)
		}
		got := Evaluate(v, pack, []models.Result{episode}, true)
		if len(got.Improvements) != 1 || got.Improvements[0].Type != "tv_structure" {
			t.Errorf("%s: season pack not recognized as an improvement over an episode: %+v", id, got)
		}
	}
}

func TestIncompleteTargetsDoNotBlockEvaluation(t *testing.T) {
	movie := models.Media{Type: "movie"}
	source := parsed("s", "Movie.2013.1080p.BluRay.x264-AAA", 9000, movie)
	v := presetPolicy(t, "ruleset_4")
	// A listing whose known metadata already places it elsewhere is not a slot question.
	elsewhere := parsed("u", "Movie 2013 2160p BluRay-NoCodec", 30000, movie)
	if got := Evaluate(v, source, []models.Result{elsewhere}, true); got.Classification != models.PolicyCoexistence {
		t.Fatalf("different-resolution listing without a codec blocked evaluation: %s %s", got.Classification, got.Reason)
	}
	// A definite occupant decides, whatever another listing lacks.
	occupant := parsed("o", "Movie.2013.1080p.BluRay.x264-BBB", 8000, movie)
	vague := parsed("v", "Movie 2013 BluRay-NoInfo", 5000, movie)
	if got := Evaluate(v, source, []models.Result{vague, occupant}, true); got.Classification == models.PolicyUnevaluated || got.Classification == models.PolicyMissingSlot || got.Classification == models.PolicyCoexistence {
		t.Fatalf("observed occupant ignored because another listing is vague: %s %s", got.Classification, got.Reason)
	}
	// A listing that may occupy the slot still prevents a candidate.
	if got := Evaluate(v, source, []models.Result{vague}, true); got.Classification != models.PolicyUnevaluated || len(got.MissingEvidence) == 0 {
		t.Fatalf("possible occupant with unknown metadata produced a conclusion: %s %s", got.Classification, got.Reason)
	}
}

func TestTargetWithoutYear(t *testing.T) {
	movie := models.Media{Type: "movie", IMDbID: "tt1767354"}
	v := presetPolicy(t, "ruleset_4")
	source := parsed("s", "Odd.Thomas.2013.1080p.BluRay.x264-AAA", 9000, movie)
	noYear := parsed("n", "Odd Thomas 1080p AMZN WEB-DL DDP5.1 H.264-GRP", 3000, movie)
	if got := Evaluate(v, source, []models.Result{noYear}, true); got.Classification != models.PolicyCoexistence {
		t.Fatalf("a yearless listing in another slot blocked evaluation: %s %s", got.Classification, got.Reason)
	}
	sameSlot := parsed("x", "Odd Thomas 1080p BluRay x264-BBB", 8000, movie)
	if got := Evaluate(v, source, []models.Result{sameSlot}, true); got.Classification != models.PolicyUnevaluated {
		t.Fatalf("a yearless possible occupant (maybe a remake) produced a conclusion: %s %s", got.Classification, got.Reason)
	}
	source.IdentityVerified, sameSlot.IdentityVerified = true, true
	source.Parsed.ContentID, sameSlot.Parsed.ContentID = "movie:imdb:tt1767354", "movie:imdb:tt1767354"
	if got := Evaluate(v, source, []models.Result{sameSlot}, true); got.Classification == models.PolicyUnevaluated {
		t.Fatalf("matching verified IDs still treated the missing year as a different film: %s", got.Reason)
	}
}

func TestHigherRevisionOfSameCorrection(t *testing.T) {
	movie := models.Media{Type: "movie"}
	v := presetPolicy(t, "ruleset_4")
	first := parsed("r1", "Movie.2013.REPACK.2160p.UHD.BluRay.x265.HDR10-GRP", 9000, movie)
	second := parsed("r2", "Movie.2013.REPACK2.2160p.UHD.BluRay.x265.HDR10-GRP", 9001, movie)
	got := Evaluate(v, second, []models.Result{first}, true)
	if len(got.Improvements) != 1 || got.Improvements[0].Type != "correction" {
		t.Fatalf("REPACK2 not an improvement over the same group's REPACK: %+v", got)
	}
	if back := Evaluate(v, first, []models.Result{second}, true); len(back.Regressions) != 1 {
		t.Fatalf("REPACK not a regression against REPACK2: %+v", back)
	}
}

func TestRuleset1EncodeSizeSlots(t *testing.T) {
	movie := models.Media{Type: "movie"}
	v := presetPolicy(t, "ruleset_1")
	big := parsed("big", "Movie.2013.1080p.BluRay.x264-BIG", 20000, movie)
	small := parsed("small", "Movie.2013.1080p.BluRay.x264-SMALL", 5000, movie)
	mid := parsed("mid", "Movie.2013.1080p.BluRay.x264-MID", 8000, movie)
	if got := Evaluate(v, mid, []models.Result{big}, true); got.Classification != models.PolicyCoexistence {
		t.Fatalf("second 1080p x264 size slot not open: %s %s", got.Classification, got.Reason)
	}
	got := Evaluate(v, mid, []models.Result{big, small}, true)
	if got.Classification == models.PolicyCoexistence || got.Classification == models.PolicyMissingSlot || len(got.CompetingResultIDs) != 1 || got.CompetingResultIDs[0] != "small" {
		t.Fatalf("third x264 encode with both size slots taken did not compete with the nearest: %+v", got)
	}
	u1 := parsed("u1", "Movie.2013.2160p.UHD.BluRay.x265-AAA", 30000, movie)
	u2 := parsed("u2", "Movie.2013.2160p.UHD.BluRay.x265-BBB", 20000, movie)
	if got := Evaluate(v, u2, []models.Result{u1}, true); got.Classification == models.PolicyCoexistence {
		t.Fatalf("size boundary split a single 2160p x265 slot: %s", got.Reason)
	}
}

func TestRuleset4BitrateTiers(t *testing.T) {
	movie := models.Media{Type: "movie"}
	v := presetPolicy(t, "ruleset_4")
	ctx := Context{RuntimeMinutes: 97}
	meesta := parsed("m", "Odd.Thomas.2013.REPACK2.1080p.BluRay.DD+.5.1.x264-MeeSta", 11470371281, movie)
	ctrlhd := parsed("c", "Odd.Thomas.2013.1080p.BluRay.DTS.x264-CtrlHD", 8530537609, movie)
	got := EvaluateWith(v, meesta, []models.Result{ctrlhd}, true, ctx)
	if got.Classification != models.PolicyCoexistence || got.SlotComponents["sizeTier"] != "transparent" {
		t.Fatalf("transparent and compact encodes did not coexist: %s %s %v", got.Classification, got.Reason, got.SlotComponents)
	}
	other := parsed("o", "Odd.Thomas.2013.1080p.BluRay.DD+.5.1.x264-OTHER", 11200000000, movie)
	if got := EvaluateWith(v, meesta, []models.Result{other}, true, ctx); got.Classification == models.PolicyCoexistence || got.Classification == models.PolicyMissingSlot {
		t.Fatalf("two transparent encodes did not compete: %s", got.Reason)
	}
	got = Evaluate(v, meesta, []models.Result{ctrlhd}, true)
	if got.Classification != models.PolicyPossibleTrump || len(got.MissingEvidence) == 0 {
		t.Fatalf("unknown runtime did not send the tier question to review: %s %s", got.Classification, got.Reason)
	}
	pack := parsed("p", "Show.S01.1080p.BluRay.x264-GRP", 30000000000, models.Media{Type: "tv"})
	if tier, _ := bitrateTier(v, pack, Context{RuntimeMinutes: 45}, 0); tier != "unknown" {
		t.Fatalf("season pack tier without an episode count: %s", tier)
	}
	if tier, _ := bitrateTier(v, pack, Context{RuntimeMinutes: 45, SeasonEpisodes: map[int]int{1: 10}}, 0); tier != "compact" {
		t.Fatalf("season pack tier: %s", tier)
	}
}

func TestEpisodesBlockedByAnyPack(t *testing.T) {
	tv := models.Media{Type: "tv"}
	pack := parsed("pack", "Show.S01.2160p.NF.WEB-DL.DDP5.1.DV.H.265-GRP", 90000, tv)
	episode := parsed("ep", "Show.S01E03.1080p.AMZN.WEB-DL.DDP5.1.H.264-OTHER", 1000, tv)
	otherSeason := parsed("s2", "Show.S02.2160p.NF.WEB-DL.DDP5.1.DV.H.265-GRP", 90000, tv)
	v := presetPolicy(t, "ruleset_4")
	if got := Evaluate(v, episode, []models.Result{pack}, true); got.Classification != models.PolicyNotEligible || len(got.CompetingResultIDs) != 1 {
		t.Fatalf("episode offered beside a pack of its season: %s %s", got.Classification, got.Reason)
	}
	if got := Evaluate(v, episode, []models.Result{otherSeason}, true); got.Classification == models.PolicyNotEligible {
		t.Fatalf("another season's pack blocked the episode: %s", got.Reason)
	}
	v.EpisodesBlockedByPack = false
	if got := Evaluate(v, episode, []models.Result{pack}, true); got.Classification == models.PolicyNotEligible {
		t.Fatalf("pack blocked an episode with the rule off: %s", got.Reason)
	}
}

func TestReviewFlagsForRulesNamesCannotSettle(t *testing.T) {
	movie := models.Media{Type: "movie"}
	existing := parsed("e", "Movie.2013.1080p.AMZN.WEB-DL.DDP5.1.H.264-AAA", 3000, movie)
	dual := parsed("d", "Movie.2013.1080p.AMZN.WEB-DL.Dual-Audio.DDP5.1.H.264-BBB", 3100, movie)
	v := presetPolicy(t, "ruleset_4")
	v.ReviewVisualQuality, v.ReviewSuperiorMaster = false, false
	if got := Evaluate(v, dual, []models.Result{existing}, true); got.Classification != models.PolicyPossibleTrump || len(got.ReviewReasons) != 1 {
		t.Fatalf("dual-audio review not raised: %s %v", got.Classification, got.ReviewReasons)
	}
	plain := parsed("p", "Movie.2013.1080p.AMZN.WEB-DL.DDP5.1.H.264-BBB", 3100, movie)
	if got := Evaluate(v, plain, []models.Result{existing}, true); got.Classification != models.PolicyNotEligible {
		t.Fatalf("occupied slot without a review rule: %s", got.Classification)
	}
	v3 := presetPolicy(t, "ruleset_3")
	v3.ReviewVisualQuality, v3.ReviewSuperiorMaster = false, false
	lossy := parsed("l", "Movie.2013.2160p.AMZN.WEB-DL.DDP5.1.Atmos.H.265-AAA", 9000, movie)
	lossless := parsed("t", "Movie.2013.2160p.AMZN.WEB-DL.TrueHD.7.1.Atmos.H.265-BBB", 12000, movie)
	if got := Evaluate(v3, lossless, []models.Result{lossy}, true); got.Classification != models.PolicyPossibleTrump {
		t.Fatalf("lossless 2160p WEB-DL slot review not raised: %s", got.Classification)
	}
}

func TestUntaggedSDRDoesNotNeedHDRReview(t *testing.T) {
	movie := models.Media{Type: "movie"}
	v := presetPolicy(t, "ruleset_4")
	original := parsed("o", "Movie.2013.1080p.AMZN.WEB-DL.DDP5.1.H.264-GRP", 3000, movie)
	repack := parsed("r", "Movie.2013.REPACK.1080p.AMZN.WEB-DL.DDP5.1.H.264-GRP", 3001, movie)
	if got := Evaluate(v, repack, []models.Result{original}, true); got.Classification != models.PolicyTrump {
		t.Fatalf("untagged 1080p H.264 REPACK sent to HDR review: %s %s", got.Classification, got.Reason)
	}
	uhdOriginal := parsed("uo", "Movie.2013.2160p.AMZN.WEB-DL.DDP5.1.H.265-GRP", 9000, movie)
	uhdRepack := parsed("ur", "Movie.2013.REPACK.2160p.AMZN.WEB-DL.DDP5.1.H.265-GRP", 9001, movie)
	if got := Evaluate(v, uhdRepack, []models.Result{uhdOriginal}, true); got.Classification != models.PolicyPossibleTrump {
		t.Fatalf("untagged 2160p H.265 skipped HDR review: %s", got.Classification)
	}
}

func TestValidateEncodeSettings(t *testing.T) {
	v := presetPolicy(t, "ruleset_1")
	for name, change := range map[string]func(*models.IndexerPolicy){
		"scope format":     func(p *models.IndexerPolicy) { p.EncodeSizeScope = []string{"1080p"} },
		"scope resolution": func(p *models.IndexerPolicy) { p.EncodeSizeScope = []string{"999p:AVC"} },
		"slots":            func(p *models.IndexerPolicy) { p.EncodeSizeSlots = 11 },
		"tier limit": func(p *models.IndexerPolicy) {
			p.EncodeBitrateTiers = []models.EncodeBitrateTier{{Resolution: "1080p", MaxVideoMbps: 0}}
		},
		"tier twice": func(p *models.IndexerPolicy) {
			p.EncodeBitrateTiers = []models.EncodeBitrateTier{{Resolution: "1080p", MaxVideoMbps: 12}, {Resolution: "1080p", MaxVideoMbps: 14}}
		},
	} {
		bad := v
		change(&bad)
		if Validate(bad) == nil {
			t.Errorf("%s accepted", name)
		}
	}
	for _, p := range Presets() {
		p.Policy.IndexerID = 1
		if err := Validate(p.Policy); err != nil {
			t.Errorf("%s invalid: %v", p.ID, err)
		}
	}
}

func TestInternalGroupsOfTheTargetTracker(t *testing.T) {
	movie := models.Media{Type: "movie"}
	outside := parsed("o", "Movie.2013.1080p.AMZN.WEB-DL.DDP5.1.H.264-AAA", 3000, movie)
	home := parsed("h", "Movie.2013.1080p.AMZN.WEB-DL.DDP5.1.H.264-HOME", 3100, movie)
	homeToo := parsed("h2", "Movie.2013.1080p.AMZN.WEB-DL.DDP5.1.H.264-HOMETOO", 3200, movie)
	v := presetPolicy(t, "ruleset_4")
	v.ReviewVisualQuality, v.ReviewSuperiorMaster = false, false
	if got := Evaluate(v, home, []models.Result{outside}, true); got.Classification != models.PolicyNotEligible {
		t.Fatalf("a group not listed for this tracker was treated as internal: %s", got.Classification)
	}
	v.InternalGroups = []string{"home", "HomeToo"}
	if got := Evaluate(v, home, []models.Result{outside}, true); got.Classification != models.PolicyCoexistence {
		t.Fatalf("internal did not get its own slot: %s %s", got.Classification, got.Reason)
	}
	if got := Evaluate(v, outside, []models.Result{home}, true); got.Classification != models.PolicyCoexistence {
		t.Fatalf("non-internal did not coexist with an internal: %s %s", got.Classification, got.Reason)
	}
	if got := Evaluate(v, homeToo, []models.Result{home}, true); got.Classification != models.PolicyNotEligible {
		t.Fatalf("two internals in one slot did not compete: %s", got.Classification)
	}
	v1 := presetPolicy(t, "ruleset_1")
	v1.ReviewVisualQuality, v1.ReviewSuperiorMaster = false, false
	v1.InternalGroups = []string{"HOME"}
	if got := Evaluate(v1, home, []models.Result{outside}, true); got.Classification != models.PolicyPossibleTrump || len(got.ReviewReasons) != 1 {
		t.Fatalf("ruleset 1 internal did not go to review: %s %v", got.Classification, got.ReviewReasons)
	}
	if got := Evaluate(v1, outside, []models.Result{home}, true); got.Classification != models.PolicyNotEligible {
		t.Fatalf("ruleset 1: a non-internal facing an internal occupant: %s", got.Classification)
	}
	bad := v1
	bad.InternalSlots = "always"
	if Validate(bad) == nil {
		t.Fatal("unknown internal treatment accepted")
	}
}

func TestBannedGroupsOfTheTargetTracker(t *testing.T) {
	movie := models.Media{Type: "movie"}
	v := presetPolicy(t, "ruleset_4")
	v.ReviewVisualQuality, v.ReviewSuperiorMaster = false, false
	v.BannedGroups = []string{"badgrp"}
	banned := parsed("b", "Movie.2013.1080p.AMZN.WEB-DL.DDP5.1.H.264-BADGRP", 3000, movie)
	good := parsed("g", "Movie.2013.1080p.AMZN.WEB-DL.DDP5.1.H.264-GOOD", 3100, movie)
	if got := Evaluate(v, banned, nil, true); got.Classification != models.PolicyNotEligible || got.Reason == "" {
		t.Fatalf("a banned group's release was offered: %s", got.Classification)
	}
	got := Evaluate(v, good, []models.Result{banned}, true)
	if got.Classification != models.PolicyTrump || len(got.Improvements) != 1 || got.Improvements[0].Type != "banned_group" {
		t.Fatalf("release over a banned group's occupant not a trump: %+v", got)
	}
	elsewhere := parsed("e", "Movie.2013.2160p.AMZN.WEB-DL.DDP5.1.H.265-BADGRP", 9000, movie)
	if got := Evaluate(v, good, []models.Result{elsewhere}, true); got.Classification != models.PolicyCoexistence {
		t.Fatalf("a banned release in another slot changed the result: %s", got.Classification)
	}
	v.BannedGroups = nil
	if got := Evaluate(v, good, []models.Result{banned}, true); got.Classification == models.PolicyTrump {
		t.Fatal("group treated as banned without a list")
	}
}

func TestBannedGroupsWithSeparatorsInTheirName(t *testing.T) {
	movie := models.Media{Type: "movie"}
	v := models.IndexerPolicy{BannedGroups: []string{"ALCAIDE_KIRA", "BEN THE MEN", "YTS.MX", "WEASLEY[HONE]", "EVO"}}
	for _, name := range []string{
		"Movie.2013.1080p.BluRay.x264-ALCAIDE_KIRA",
		"Movie.2013.1080p.WEBRip.x264-BEN.THE.MEN",
		"Movie 2013 1080p WEBRip x264-BEN THE MEN",
		"Movie.2013.1080p.BluRay.x264-YTS.MX.mkv",
		"Movie.2013.1080p.WEB-DL.DDP5.1.H.264-WEASLEY[HONE]",
		"Movie.2013.1080p.WEBRip.x264-evo",
	} {
		if !Banned(v, parsed("b", name, 1, movie)) {
			t.Errorf("%s not recognized as banned", name)
		}
	}
	for _, name := range []string{
		"Movie.2013.1080p.BluRay.x264-MX",
		"Movie.2013.1080p.BluRay.x264-KIRA",
		"Movie.2013.1080p.WEB-DL.DDP5.1.H.264-WEASLEY",
		"Movie.2013.1080p.BluRay.x264-EVOLVE",
	} {
		if Banned(v, parsed("a", name, 1, movie)) {
			t.Errorf("%s wrongly banned", name)
		}
	}
}

func TestMeasuredSourceBitrateSettlesTheTier(t *testing.T) {
	movie := models.Media{Type: "movie"}
	v := presetPolicy(t, "ruleset_4")
	hallowed := parsed("h", "Babylon.2022.BluRay.1080p.DDP.Atmos.5.1.x264-hallowed", 18556478093, movie)
	hidt := parsed("d", "Babylon 2022 1080p BluRay DD+7.1 x264-HiDt", 25436585000, movie)
	estimated := Context{RuntimeMinutes: 189}
	if got := EvaluateWith(v, hallowed, []models.Result{hidt}, true, estimated); got.Classification == models.PolicyCoexistence {
		t.Fatalf("estimate at the limit should not settle the tier: %s", got.Reason)
	}
	measured := Context{RuntimeMinutes: 189, SourceVideoMbps: 11.996159}
	got := EvaluateWith(v, hallowed, []models.Result{hidt}, true, measured)
	if got.Classification != models.PolicyCoexistence || got.SlotComponents["sizeTier"] != "compact" {
		t.Fatalf("measured 12 Mbps compact encode did not coexist with a transparent one: %s %s %v", got.Classification, got.Reason, got.SlotComponents)
	}
}

func TestUpgradeReasonsKeepTheirReviewChecks(t *testing.T) {
	movie := models.Media{Type: "movie"}
	v := presetPolicy(t, "ruleset_4")
	ctx := Context{RuntimeMinutes: 107}
	orbit := parsed("o", "The.Sixth.Sense.1999.1080p.UHD.BluRay.DDP5.1.DV.HDR.x265-ORBiT", 21743484518, movie)
	hidt := parsed("h", "The Sixth Sense 1999 Hybrid 1080p UHD BluRay DDP5.1 DV HDR10P x265-HiDt", 15848700000, movie)
	upgradeOf := func(e models.PolicyEvaluation) models.PolicyTargetResult {
		for _, r := range e.TargetResults {
			if len(r.Upgrade) > 0 {
				return r
			}
		}
		t.Fatalf("no upgrade reasons: %+v", e.TargetResults)
		return models.PolicyTargetResult{}
	}
	got := upgradeOf(EvaluateWith(v, orbit, []models.Result{hidt}, true, ctx))
	if got.Upgrade[0].Type != "hdr" || got.UpgradeReview == "" {
		t.Fatalf("HDR-only upgrade from another group skipped the comparability review: %+v", got)
	}
	v.ReviewHDRComparability = false
	if got = upgradeOf(EvaluateWith(v, orbit, []models.Result{hidt}, true, ctx)); got.UpgradeReview != "" {
		t.Fatalf("review note without the review setting: %+v", got)
	}
	original := parsed("r1", "Movie.2013.1080p.AMZN.WEB-DL.DDP5.1.H.264-GRP", 3000, movie)
	repack := parsed("r2", "Movie.2013.REPACK.1080p.AMZN.WEB-DL.DDP5.1.H.264-GRP", 3001, movie)
	if got = upgradeOf(Evaluate(presetPolicy(t, "ruleset_4"), original, []models.Result{repack}, true)); got.UpgradeReview != "" {
		t.Fatalf("same-group REPACK upgrade sent to review: %+v", got)
	}
}
