package clustering

import (
	"ravenseed/internal/matching"
	"ravenseed/internal/models"
	"ravenseed/internal/releases"
	"slices"
	"testing"
)

// Family matching keeps a missing service as a difference unless identical
// bytes tie both names to one file.
func TestMissingServiceWithIdenticalBytesRemainsPossible(t *testing.T) {
	media := models.Media{Type: "tv", IMDbID: "tt5396394"}
	a := advancedResult(22, "American Housewife S01 1080p AMZN WEB-DL DD 5.1 H.264-NTb", media, releases.DefaultRules())
	b := advancedResult(12, "American Housewife S01 1080p WEB-DL DD 5.1 H.264-NTb", media, releases.DefaultRules())
	a.Size, b.Size = 20251886141, 20251886141
	if c, e := compare(a, b); c != "POSSIBLE" || !slices.Contains(e, "missing_service") {
		t.Fatalf("missing service promoted despite different naming: %s %v", c, e)
	}
	if !matching.ObservedEquivalent(a, b) {
		t.Fatal("missing service hid an existing listing with identical bytes")
	}
	c := b
	c.Parsed.Service = "NF"
	if matching.ObservedEquivalent(a, c) {
		t.Fatal("different services treated as one listing")
	}
	req := request()
	req.TargetIndexerID = 12
	for _, rows := range [][]models.Result{{a, b}, {b, a}} {
		fs := build(rows, req, []models.IndexerProgress{{IndexerID: 12, Status: "completed"}})
		if len(fs) != 1 || fs[0].Coverage != 2 || len(fs[0].Results) != 2 {
			t.Fatalf("one file with and without a service tag not merged: %+v", fs)
		}
		if fs[0].TargetStatus.Candidate || !fs[0].TargetStatus.ExactMatchFound {
			t.Fatalf("the target's copy of the same file not recognized: %+v", fs[0].TargetStatus)
		}
	}
	// Without identical bytes, a missing service keeps the names apart.
	b.Size++
	fs := build([]models.Result{a, b}, req, []models.IndexerProgress{{IndexerID: 12, Status: "completed"}})
	if len(fs) != 2 {
		t.Fatal("missing service merged on the name alone")
	}
	for _, f := range fs {
		if f.Coverage != 1 || f.Confidence != "POSSIBLE" || len(f.RelatedFamilies) != 1 || f.RelatedFamilies[0].Confidence != "POSSIBLE" {
			t.Fatalf("incorrect review family: %+v", f)
		}
		if f.TargetStatus.Candidate {
			t.Fatal("uncertain target promoted to upload candidate")
		}
	}
}
