package matching

import (
	"slices"
	"testing"

	"ravenseed/internal/models"
	"ravenseed/internal/releases"
)

// compare matches two results with the default matching rules.
func compare(a, b models.Result) (string, []string) {
	return CompareWithRules(a, b, releases.DefaultRules())
}

func parsedResult(title string, size int64, media models.Media) models.Result {
	r := models.Result{Title: title, Size: size}
	releases.Parse(&r, media)
	return r
}

func TestCompareRemakesAndRelatedReleases(t *testing.T) {
	movie := models.Media{Type: "movie"}
	for _, tt := range []struct {
		name, a, b string
		want       string
	}{
		{"remake", "Dune.1984.1080p.BluRay.x264-AAA", "Dune.2021.1080p.BluRay.x264-AAA", "REJECTED"},
		{"other group", "Dune.2021.1080p.BluRay.x264-AAA", "Dune.2021.1080p.BluRay.x264-BBB", "REJECTED"},
		{"other audio", "Dune.2021.1080p.BluRay.DTS.x264-AAA", "Dune.2021.1080p.BluRay.FLAC.x264-AAA", "RELATED"},
		{"missing audio", "Dune.2021.1080p.BluRay.DTS.x264-AAA", "Dune.2021.1080p.BluRay.x264-AAA", "POSSIBLE"},
	} {
		got, evidence := compare(parsedResult(tt.a, 1000, movie), parsedResult(tt.b, 2000, movie))
		if got != tt.want {
			t.Errorf("%s: %s %v, want %s", tt.name, got, evidence, tt.want)
		}
	}
	// Verified IDs settle a year that drifts between trackers.
	a, b := parsedResult("Dune.2021.1080p.BluRay.DTS.x264-AAA", 1000, movie), parsedResult("Dune.2020.1080p.BluRay.FLAC.x264-AAA", 2000, movie)
	a.Parsed.ContentID, b.Parsed.ContentID = "movie:imdb:tt1160419", "movie:imdb:tt1160419"
	if got, evidence := compare(a, b); got != "RELATED" || !slices.Contains(evidence, "different_release_metadata") {
		t.Fatalf("verified year drift: %s %v", got, evidence)
	}
}

func TestCompareFingerprintedMatchesCompare(t *testing.T) {
	movie := models.Media{Type: "movie", IMDbID: "tt0133093"}
	rules := releases.DefaultRules()
	titles := []string{"The.Matrix.1999.1080p.BluRay.x264-GRP", "The.Matrix.1999.1080p.BluRay.DTS.x264-GRP", "The.Matrix.1999.2160p.UHD.BluRay.x265-GRP"}
	for _, x := range titles {
		for _, y := range titles {
			a, b := parsedResult(x, 1000, movie), parsedResult(y, 1000, movie)
			c1, e1 := CompareWithRules(a, b, rules)
			c2, e2 := CompareFingerprinted(a, b, Fingerprint(a.Parsed), Fingerprint(b.Parsed), rules)
			if c1 != c2 || !slices.Equal(e1, e2) {
				t.Fatalf("%s vs %s: %s %v != %s %v", x, y, c1, e1, c2, e2)
			}
		}
	}
}
