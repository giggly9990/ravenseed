package clustering

import (
	"ravenseed/internal/models"
	"ravenseed/internal/releases"
	"testing"
)

// Tracker titles follow each site's naming rules; one uploader's release can
// carry a language tag on one site and an alternate title on another.
func TestTargetTitleConventionsDoNotHideTheSameFile(t *testing.T) {
	tv := models.Media{Type: "tv"}
	listing := func(id, indexer int, title string) models.Result {
		r := models.Result{ID: string(rune('a' + id)), IndexerID: indexer, Title: title, Size: 11790977098}
		releases.Parse(&r, tv)
		return r
	}
	req := models.SearchRequest{Media: tv, IndexerIDs: []int{1, 2, 3}, TargetIndexerID: 3}
	req.Filters.MinimumCoverage = 2
	results := []models.Result{
		listing(0, 1, "Tale of the Nine Tailed S01 KOREAN 1080p AMZN WEB-DL DD+ 2.0 H.264-ARCHIE"),
		listing(1, 2, "Tale.of.the.Nine.Tailed.S01.KOREAN.1080p.AMZN.WEB-DL.DDP2.0.H.264-ARCHIE"),
		listing(2, 3, "Tale of the Nine Tailed AKA Gumihodyeon S01 1080p AMZN WEB-DL DD+ 2.0 H.264-ARCHIE"),
	}
	for _, f := range build(results, req, []models.IndexerProgress{{IndexerID: 3, Status: "completed"}}) {
		if f.TargetStatus.Candidate {
			t.Fatalf("release already on the target offered as missing: %+v", f.TargetStatus)
		}
		if f.Coverage == 2 && !f.TargetStatus.ExactMatchFound {
			t.Fatalf("same file on target not recognized: %+v", f.TargetStatus)
		}
	}
	changed := results
	changed[2].Size++
	for _, f := range build(changed, req, []models.IndexerProgress{{IndexerID: 3, Status: "completed"}}) {
		if f.Coverage == 2 && f.TargetStatus.ExactMatchFound {
			t.Fatal("different bytes on target counted as the same file")
		}
	}
}

func TestSameFileUnderTrackerNamesSharesOneFamily(t *testing.T) {
	tv := models.Media{Type: "tv", TMDbID: 117376}
	listing := func(id string, indexer int, title string, size int64) models.Result {
		r := models.Result{ID: id, IndexerID: indexer, Title: title, Size: size, IdentityVerified: true}
		releases.Parse(&r, tv)
		return r
	}
	req := models.SearchRequest{Media: tv, IndexerIDs: []int{1, 2, 3, 4}, TargetIndexerID: 4}
	req.Filters.MinimumCoverage = 2
	results := []models.Result{
		listing("ath", 1, "Tale of the Nine Tailed S01 KOREAN 1080p AMZN WEB-DL DD+ 2.0 H.264-ARCHIE", 11790977098),
		listing("blu", 2, "Tale of the Nine Tailed AKA Gumihodyeon S01 1080p AMZN WEB-DL DD+ 2.0 H.264-ARCHIE", 11790977098),
		// Same bytes, another group's tag: never the same release.
		listing("re", 3, "Tale of the Nine Tailed S01 1080p AMZN WEB-DL DD+ 2.0 H.264-RETAG", 11790977098),
	}
	families := build(results, req, []models.IndexerProgress{{IndexerID: 4, Status: "completed"}})
	var archie *models.Family
	for i := range families {
		for _, r := range families[i].Results {
			if r.ID == "ath" {
				archie = &families[i]
			}
		}
	}
	if archie == nil || len(archie.Results) != 2 || archie.Coverage != 2 {
		t.Fatalf("one release under two tracker titles not merged: %+v", families)
	}
	for _, r := range archie.Results {
		if r.ID == "re" {
			t.Fatal("a different group's tag merged on identical bytes")
		}
		if r.Confidence != "EXACT" {
			t.Fatalf("%s confidence %s %v", r.ID, r.Confidence, r.Evidence)
		}
	}
	if archie.Confidence != "EXACT" || !archie.TargetStatus.Candidate {
		t.Fatalf("merged family on two indexers and missing on target: %+v", archie.TargetStatus)
	}
	if len(families) != 2 {
		t.Fatalf("expected the ARCHIE family and the RETAG family: %d", len(families))
	}
	again := build([]models.Result{results[1], results[2], results[0]}, req, []models.IndexerProgress{{IndexerID: 4, Status: "completed"}})
	ids := map[string]bool{}
	for _, f := range families {
		ids[f.ID] = true
	}
	for _, f := range again {
		if !ids[f.ID] {
			t.Fatal("merged family IDs depend on result order")
		}
	}
}
