package search

import (
	"ravenseed/internal/models"
	"ravenseed/internal/releases"
	"testing"
)

func TestPossibleUpgradesSkipClearlyWorseReleases(t *testing.T) {
	movie := models.Media{Type: "movie"}
	source := func(title string, size int64) models.Result {
		r := models.Result{Title: title, Size: size}
		releases.Parse(&r, movie)
		return r
	}
	occupied := func(title string, size int64, group string) models.PolicyEvaluation {
		return models.PolicyEvaluation{Classification: models.PolicyNotEligible, TargetResults: []models.PolicyTargetResult{{ID: "t", Title: title, Size: size, Group: group, Relation: "same_slot"}}}
	}
	for _, tc := range []struct {
		name, source, target, group string
		sourceSize, targetSize      int64
		want                        string
	}{
		{"smaller encode", "Casper.1995.1080p.BluRay.DTS.x264-HDMaNiAcS.mkv", "Casper.1995.1080p.BluRay.DD+.5.1.x264-playHD.mkv", "PLAYHD", 16353316250, 13722494156, ""},
		{"remux of the same size", "The.Death.of.Stalin.2017.BluRay.1080p.DTS-HD.MA.5.1.AVC.REMUX-FraMeSToR.mkv", "The.Death.of.Stalin.2017.REMUX.1080p.Blu-ray.AVC.DTS-HD.MA.5.1-LEGi0N.mkv", "LEGI0N", 25598221312, 25598221312, UpgradePossible},
		{"same-service WEB-DL", "Code.3.2025.1080p.AMZN.WEB-DL.DDP5.1.H.264-Kitsune.mkv", "Code.3.2025.1080p.AMZN.WEB-DL.DDP5.1.H.264-BYNDR.mkv", "BYNDR", 6120328396, 6270652170, UpgradePossible},
		{"lower-bitrate WEB-DL stream", "300.2007.1080p.AMZN.WEB-DL.DDP.5.1.H.264-Yuki.mkv", "300.2007.1080p.AMZN.WEB-DL.DDP5.1.H.264-BLOOM.mkv", "BLOOM", 12165297029, 8869104762, ""},
		{"slightly smaller WEB-DL", "Movie.2013.1080p.AMZN.WEB-DL.DDP5.1.H.264-AAA", "Movie.2013.1080p.AMZN.WEB-DL.DDP5.1.H.264-BBB", "BBB", 10000, 9300, UpgradePossible},
		{"larger encode", "Movie.2013.1080p.BluRay.DTS.x264-AAA", "Movie.2013.1080p.BluRay.DTS.x264-BBB", "BBB", 10000, 12000, UpgradePossible},
		{"lossless to lossy audio", "Movie.2013.1080p.AMZN.WEB-DL.TrueHD.5.1.H.264-AAA", "Movie.2013.1080p.AMZN.WEB-DL.DDP5.1.H.264-BBB", "BBB", 10000, 10000, ""},
		{"fewer channels", "Movie.2013.1080p.AMZN.WEB-DL.DDP5.1.H.264-AAA", "Movie.2013.1080p.AMZN.WEB-DL.AAC2.0.H.264-BBB", "BBB", 10000, 10000, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			level, _ := upgradeOffer(models.IndexerPolicy{}, source(tc.source, tc.sourceSize), occupied(tc.target, tc.targetSize, tc.group))
			if level != tc.want {
				t.Fatalf("got %q, want %q", level, tc.want)
			}
		})
	}
	// A rule-backed upgrade is never suppressed by these hints.
	e := occupied("Casper.1995.REPACK.1080p.BluRay.DD+.5.1.x264-HDMaNiAcS.mkv", 1000, "HDMANIACS")
	e.TargetResults[0].Upgrade = []models.PolicyImprovement{{Type: "correction", From: "original", To: "REPACK"}}
	if level, _ := upgradeOffer(models.IndexerPolicy{}, source("Casper.1995.1080p.BluRay.DTS.x264-HDMaNiAcS.mkv", 16353316250), e); level != UpgradeObjective {
		t.Fatalf("rule-backed upgrade suppressed: %q", level)
	}
}
