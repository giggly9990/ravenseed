package clustering

import (
	"fmt"
	"ravenseed/internal/matching"
	"ravenseed/internal/models"
	"ravenseed/internal/ranking"
	"ravenseed/internal/releases"
	"testing"
)

const title = "The.Matrix.1999.2160p.UHD.BluRay.REMUX.HEVC.DV.HDR.TrueHD.7.1-FraMeSToR"

func result(id int, size int64, name string) models.Result {
	r := models.Result{ID: fmt.Sprintf("%d-%d-%s", id, size, name), IndexerID: id, Title: name, Size: size, IdentityVerified: true}
	releases.Parse(&r, models.Media{Type: "movie", IMDbID: "tt0133093"})
	return r
}

// build clusters results with the default matching rules.
func build(results []models.Result, req models.SearchRequest, progress []models.IndexerProgress) []models.Family {
	return BuildWithRules(results, req, progress, releases.DefaultRules())
}

// compare matches two results with the default matching rules.
func compare(a, b models.Result) (string, []string) {
	return matching.CompareWithRules(a, b, releases.DefaultRules())
}

func request() models.SearchRequest {
	r := models.SearchRequest{Media: models.Media{Type: "movie", IMDbID: "tt0133093"}, IndexerIDs: []int{1, 2, 3}, TargetIndexerID: 3}
	r.Filters.MinimumCoverage = 2
	return r
}
func TestExactCoverageAndCandidate(t *testing.T) {
	r1, r2 := result(1, 10000000, title), result(2, 10000000, title)
	duplicate := r1
	duplicate.ID = "duplicate"
	fs := build([]models.Result{r1, r2, duplicate}, request(), []models.IndexerProgress{{IndexerID: 3, Status: "completed"}})
	if len(fs) != 1 || fs[0].Coverage != 2 || fs[0].Confidence != "EXACT" || !fs[0].TargetStatus.Candidate {
		t.Fatalf("unexpected family: %+v", fs)
	}
	if len(fs[0].Variants) != 1 {
		t.Fatal("exact bytes should share a variant")
	}
}
func TestSizeVariants(t *testing.T) {
	fs := build([]models.Result{result(1, 10000000, title), result(2, 10000001, title)}, request(), nil)
	if len(fs) != 1 || len(fs[0].Variants) != 2 || fs[0].Coverage != 1 || fs[0].Confidence != "STRONG" {
		t.Fatalf("unexpected variants: %+v", fs)
	}
}
func TestHardMismatches(t *testing.T) {
	for _, other := range []string{"The.Matrix.1999.1080p.UHD.BluRay.REMUX.HEVC.DV.HDR.TrueHD.7.1-FraMeSToR", "The.Matrix.1999.2160p.UHD.BluRay.x265.DV.HDR.TrueHD.7.1-FraMeSToR", "The.Matrix.1999.2160p.UHD.BluRay.REMUX.HEVC.DV.HDR.TrueHD.7.1-TRiToN", "The.Matrix.1999.Extended.2160p.UHD.BluRay.REMUX.HEVC.DV.HDR.TrueHD.7.1-FraMeSToR"} {
		a, b := result(1, 10000, title), result(2, 10001, other)
		if c, _ := compare(a, b); c != "REJECTED" {
			t.Errorf("expected rejection for %s, got %s", other, c)
		}
		if fs := build([]models.Result{a, b}, request(), nil); len(fs) != 2 {
			t.Errorf("merged %s", other)
		}
	}
	for _, pair := range [][2]string{{"Example.Show.S01.1080p.WEB-DL.H.264-GROUP", "Example.Show.S01E01.1080p.WEB-DL.H.264-GROUP"}, {"The.Matrix.1999.1080p.WEB-DL.H.264-GROUP", "The.Matrix.1999.1080p.WEBRip.H.264-GROUP"}} {
		if c, _ := compare(result(1, 10000, pair[0]), result(2, 10000, pair[1])); c != "REJECTED" {
			t.Fatalf("expected rejection: %s", c)
		}
	}
}
func TestMissingSizeAndIdentityCannotBeExact(t *testing.T) {
	for _, size := range []int64{0, -1, 1000} {
		a, b := result(1, size, title), result(2, size, title)
		if size > 0 {
			a.IdentityVerified = false
			b.IdentityVerified = false
		}
		fs := build([]models.Result{a, b}, request(), []models.IndexerProgress{{IndexerID: 3, Status: "completed"}})
		if fs[0].Confidence == "EXACT" || fs[0].TargetStatus.Candidate {
			t.Fatal("uncertain metadata produced high confidence")
		}
		if size <= 0 && fs[0].Coverage != 0 {
			t.Fatal("unknown size counted as coverage")
		}
	}
}
func TestTargetFailureAndVariants(t *testing.T) {
	for _, status := range []string{"failed", "queued", "running"} {
		fs := build([]models.Result{result(1, 1000, title), result(2, 1000, title)}, request(), []models.IndexerProgress{{IndexerID: 3, Status: status}})
		if fs[0].TargetStatus.Candidate || fs[0].TargetStatus.Evaluated {
			t.Fatal("failed target marked missing")
		}
	}
	for _, size := range []int64{1000, 1001, 2000} {
		fs := build([]models.Result{result(1, 1000, title), result(2, 1000, title), result(3, size, title)}, request(), []models.IndexerProgress{{IndexerID: 3, Status: "completed"}})
		if fs[0].TargetStatus.Candidate != (size != 1000) {
			t.Fatal("target status did not distinguish exact-size variants")
		}
	}
}
func TestRankingKeepsConfidenceAbovePopularity(t *testing.T) {
	fs := []models.Family{{ID: "possible", Confidence: "POSSIBLE", Coverage: 10, Seeders: 10000}, {ID: "exact", Confidence: "EXACT", Coverage: 2}}
	ranking.Sort(fs, "coverage")
	if fs[0].ID != "exact" {
		t.Fatal("popularity outranked confidence")
	}
}
func TestGroupCaseNormalization(t *testing.T) {
	a := result(1, 10000, "The.Matrix.1999.1080p.BluRay.x264-FraMeSToR")
	b := result(2, 10000, "The.Matrix.1999.1080p.BluRay.x264-FRAMESTOR")
	if len(build([]models.Result{a, b}, request(), nil)) != 1 {
		t.Fatal("group casing split a family")
	}
}

func TestVariantComparedWithAnotherIndexer(t *testing.T) {
	a, b, variant := result(1, 10000000, title), result(2, 10000000, title), result(2, 10000001, title)
	a.ID, b.ID, variant.ID = "z", "a", "b"
	fs := build([]models.Result{a, b, variant}, request(), nil)
	for _, r := range fs[0].Results {
		if r.ID == variant.ID && r.Confidence != "STRONG" {
			t.Fatalf("variant lost cross-indexer comparison: %s", r.Confidence)
		}
	}
}
