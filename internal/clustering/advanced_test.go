package clustering

import (
	"fmt"
	"slices"
	"testing"

	"ravenseed/internal/matching"
	"ravenseed/internal/models"
	"ravenseed/internal/releases"
)

func advancedResult(id int, title string, media models.Media, rules models.MatchingRules) models.Result {
	r := models.Result{ID: fmt.Sprint(id), IndexerID: id, Title: title, Size: 10000000, IdentityVerified: true}
	releases.ParseWithRules(&r, media, rules)
	return r
}

func TestAdvancedEquivalentNames(t *testing.T) {
	movie := models.Media{Type: "movie", IMDbID: "tt0133093"}
	tv := models.Media{Type: "tv", TMDbID: 1399}
	rules := releases.DefaultRules()
	for _, tt := range []struct {
		name, a, b string
		media      models.Media
	}{
		{"alternate_titles", "The.Matrix.1999.1080p.BluRay.x264-GROUP", "La.Matrice.1999.1080p.BluRay.x264-GROUP", movie},
		{"web_dl_alias", "The.Matrix.1999.1080p.WEB-DL.H.264-GROUP", "The.Matrix.1999.1080p.WEB.DL.H.264-GROUP", movie},
		{"web_service_alias", "The.Matrix.1999.1080p.NF.WEB-DL.H.264-GROUP", "The.Matrix.1999.1080p.NETFLIX.WEB-DL.H.264-GROUP", movie},
		{"site_suffix", "The.Matrix.1999.1080p.BluRay.x264-GROUP", "The.Matrix.1999.1080p.BluRay.x264-GROUP [tracker.example]", movie},
		{"group_at_site", "The.Matrix.1999.1080p.BluRay.x264-GROUP", "The.Matrix.1999.1080p.BluRay.x264-GROUP@tracker.example", movie},
		{"episode_style", "Example.Show.S01E02.1080p.WEB-DL.H.264-GROUP", "Example.Show.1x02.1080p.WEB-DL.H.264-GROUP", tv},
		{"named_episode", "Example.Show.S01E02.1080p.WEB-DL.H.264-GROUP", "Example.Show.Season.1.Episode.2.1080p.WEB-DL.H.264-GROUP", tv},
		{"episode_range", "Example.Show.S01E01E02.1080p.WEB-DL.H.264-GROUP", "Example.Show.S01E01-E02.1080p.WEB-DL.H.264-GROUP", tv},
		{"season_pack", "Example.Show.S01.1080p.WEB-DL.H.264-GROUP", "Example.Show.Season.1.1080p.WEB-DL.H.264-GROUP", tv},
		{"season_range", "Example.Show.S01-S03.1080p.WEB-DL.H.264-GROUP", "Example.Show.S01.S02.S03.1080p.WEB-DL.H.264-GROUP", tv},
		{"repack_number", "The.Matrix.1999.REPACK2.1080p.BluRay.x264-GROUP", "The.Matrix.1999.REPACK.2.1080p.BluRay.x264-GROUP", movie},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a, b := advancedResult(1, tt.a, tt.media, rules), advancedResult(2, tt.b, tt.media, rules)
			if c, e := compare(a, b); c != "EXACT" {
				t.Fatalf("%s %v\na=%+v\nb=%+v", c, e, a.Parsed, b.Parsed)
			}
			fs := BuildWithRules([]models.Result{a, b}, request(), nil, rules)
			if len(fs) != 1 || fs[0].Coverage != 2 {
				t.Fatalf("families: %+v", fs)
			}
		})
	}
}

func TestAdvancedHardBoundaries(t *testing.T) {
	movie := models.Media{Type: "movie", IMDbID: "tt0133093"}
	tv := models.Media{Type: "tv", TMDbID: 1399}
	rules := releases.DefaultRules()
	for _, tt := range []struct {
		name, a, b string
		media      models.Media
	}{
		{"repack_original", "The.Matrix.1999.1080p.BluRay.x264-GROUP", "The.Matrix.1999.REPACK.1080p.BluRay.x264-GROUP", movie},
		{"repack_revision", "The.Matrix.1999.REPACK.1080p.BluRay.x264-GROUP", "The.Matrix.1999.REPACK2.1080p.BluRay.x264-GROUP", movie},
		{"proper_repack", "The.Matrix.1999.PROPER.1080p.BluRay.x264-GROUP", "The.Matrix.1999.REPACK.1080p.BluRay.x264-GROUP", movie},
		{"rerip_repack", "The.Matrix.1999.RERIP.1080p.BluRay.x264-GROUP", "The.Matrix.1999.REPACK.1080p.BluRay.x264-GROUP", movie},
		{"hybrid_original", "The.Matrix.1999.2160p.UHD.BluRay.REMUX.HEVC.DV.HDR-GROUP", "The.Matrix.1999.HYBRID.2160p.UHD.BluRay.REMUX.HEVC.DV.HDR-GROUP", movie},
		{"different_web_services", "The.Matrix.1999.1080p.NF.WEB-DL.H.264-GROUP", "The.Matrix.1999.1080p.AMZN.WEB-DL.H.264-GROUP", movie},
		{"pack_episode", "Example.Show.S01.1080p.WEB-DL.H.264-GROUP", "Example.Show.S01E01.1080p.WEB-DL.H.264-GROUP", tv},
		{"episode_range_single", "Example.Show.S01E01E02.1080p.WEB-DL.H.264-GROUP", "Example.Show.S01E01.1080p.WEB-DL.H.264-GROUP", tv},
		{"season_range_single", "Example.Show.S01-S03.1080p.WEB-DL.H.264-GROUP", "Example.Show.S01.1080p.WEB-DL.H.264-GROUP", tv},
		{"absolute_season", "[SubsPlease] Example Show - 13 (1080p WEB-DL HEVC)", "[SubsPlease] Example Show S02E01 (1080p WEB-DL HEVC)", tv},
		{"anime_revision", "[SubsPlease] Example Show - 01 (1080p WEB-DL HEVC)", "[SubsPlease] Example Show - 01v2 (1080p WEB-DL HEVC)", tv},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a, b := advancedResult(1, tt.a, tt.media, rules), advancedResult(2, tt.b, tt.media, rules)
			// Different releases never share exact bytes; identical bytes would
			// rightly merge them as one file.
			b.Size++
			if c, _ := compare(a, b); c == "EXACT" || c == "STRONG" {
				t.Fatalf("unsafe match: %s\na=%+v\nb=%+v", c, a.Parsed, b.Parsed)
			}
			if len(BuildWithRules([]models.Result{a, b}, request(), nil, rules)) != 2 {
				t.Fatal("hard boundary merged")
			}
		})
	}
}

func TestAlternateTitlesRequireVerifiedIdentity(t *testing.T) {
	rules := releases.DefaultRules()
	media := models.Media{Type: "movie", IMDbID: "tt0133093"}
	a := advancedResult(1, "The.Matrix.1999.1080p.BluRay.x264-GROUP", media, rules)
	b := advancedResult(2, "La.Matrice.1999.1080p.BluRay.x264-GROUP", media, rules)
	if c, e := compare(a, b); c != "EXACT" || !slices.Contains(e, "alternate_titles_same_verified_id") {
		t.Fatalf("missing alternate-title evidence: %s %v", c, e)
	}
	b.IdentityVerified = false
	releases.ParseWithRules(&b, media, rules)
	if matching.Fingerprint(a.Parsed) == matching.Fingerprint(b.Parsed) {
		t.Fatal("unverified title merged")
	}
	b.IdentityVerified = true
	releases.ParseWithRules(&b, models.Media{Type: "movie", IMDbID: "tt9999999"}, rules)
	if c, _ := compare(a, b); c != "REJECTED" {
		t.Fatal("different identities matched")
	}
	rules.MatchAlternateTitles = false
	releases.ParseWithRules(&a, media, rules)
	releases.ParseWithRules(&b, media, rules)
	if matching.Fingerprint(a.Parsed) == matching.Fingerprint(b.Parsed) {
		t.Fatal("disabled alternate title rule applied")
	}
}

func TestExplicitGroupAliasAndIgnoredTag(t *testing.T) {
	rules := releases.DefaultRules()
	rules.GroupAliases = map[string]string{"OldGroup": "GROUP"}
	rules.IgnoredTags = []string{"CUSTOMTAG"}
	rules, err := releases.NormalizeRules(&rules)
	if err != nil {
		t.Fatal(err)
	}
	media := models.Media{Type: "movie", IMDbID: "tt0133093"}
	a := advancedResult(1, "The.Matrix.1999.1080p.BluRay.x264-GROUP", media, rules)
	b := advancedResult(2, "The.Matrix.1999.CUSTOMTAG.1080p.BluRay.x264-OldGroup [CUSTOMTAG]", media, rules)
	if c, _ := compare(a, b); c != "EXACT" {
		t.Fatalf("explicit aliases did not match: %+v %+v", a.Parsed, b.Parsed)
	}
	if b.Parsed.DisplayGroup != "OldGroup" || !slices.Contains(b.NormalizationEvidence, "explicit_group_alias") || !slices.Contains(b.NormalizationEvidence, "ignored_cosmetic_tag:CUSTOMTAG") {
		t.Fatalf("missing normalization provenance: %+v", b)
	}
	if !slices.Contains([]string{b.Title}, "The.Matrix.1999.CUSTOMTAG.1080p.BluRay.x264-OldGroup [CUSTOMTAG]") {
		t.Fatal("original title changed")
	}
}

func TestAnimeAbsoluteMapping(t *testing.T) {
	rules := releases.DefaultRules()
	rules.EpisodeMappings = []models.EpisodeMapping{{TMDbID: 1399, Absolute: 13, Season: 2, Episode: 1}}
	media := models.Media{Type: "tv", TMDbID: 1399}
	a := advancedResult(1, "[SubsPlease] Example Show - 13 (1080p WEB-DL HEVC) [AB12CD34].mkv", media, rules)
	b := advancedResult(2, "[SubsPlease] Example Show S02E01 (1080p WEB-DL HEVC)", media, rules)
	if c, _ := compare(a, b); c != "EXACT" {
		t.Fatalf("explicit episode mapping did not match: %s\na=%+v\nb=%+v", c, a.Parsed, b.Parsed)
	}
	if !slices.Contains(a.NormalizationEvidence, "explicit_absolute_episode_mapping") {
		t.Fatal("missing mapping evidence")
	}
	season, episode := 2, 1
	if !releases.InScope(a.Parsed, models.Media{Type: "tv", Season: &season, Episode: &episode}) {
		t.Fatal("mapped episode excluded from search")
	}
}

func TestConfigurableSizeTolerancePreservesVariants(t *testing.T) {
	rules := releases.DefaultRules()
	rules.SizeTolerancePercent = 0.5
	media := models.Media{Type: "movie", IMDbID: "tt0133093"}
	a, b := advancedResult(1, title, media, rules), advancedResult(2, title, media, rules)
	b.Size += 40000
	if c, _ := matching.CompareWithRules(a, b, rules); c != "STRONG" {
		t.Fatalf("configured tolerance ignored: %s", c)
	}
	fs := BuildWithRules([]models.Result{a, b}, request(), nil, rules)
	if len(fs) != 1 || len(fs[0].Variants) != 2 || fs[0].Coverage != 1 {
		t.Fatal("tolerance merged byte variants")
	}
	rules.SizeTolerancePercent = 0
	if c, _ := matching.CompareWithRules(a, b, rules); c != "POSSIBLE" {
		t.Fatal("disabled tolerance applied")
	}
}
