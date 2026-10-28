package clustering

import (
	"ravenseed/internal/models"
	"testing"
)

func TestVariantTargetStatus(t *testing.T) {
	rows := []models.Result{result(1, 1000, title), result(2, 1000, title), result(2, 2000, title), result(3, 2000, title)}
	fs := build(rows, request(), []models.IndexerProgress{{IndexerID: 3, Status: "completed"}})
	if len(fs) != 1 || !fs[0].TargetStatus.Candidate {
		t.Fatal("missing variant was hidden by another target variant")
	}
	for _, v := range fs[0].Variants {
		if v.TargetStatus == nil || v.TargetStatus.Candidate != (v.Size == 1000) || v.TargetStatus.ExactMatchFound != (v.Size == 2000) {
			t.Fatalf("incorrect variant state: %+v", v)
		}
	}
}

func TestMissingMetadataRelationshipDoesNotInflateCoverage(t *testing.T) {
	a, b := result(1, 1000, title), result(2, 1000, title)
	b.Parsed.AudioCodec = ""
	if fs := build([]models.Result{a, b}, request(), nil); len(fs) != 1 || fs[0].Coverage != 2 {
		t.Fatalf("identical bytes with an unnamed audio format not merged: %+v", fs)
	}
	b.Size++
	fs := build([]models.Result{a, b}, request(), nil)
	if len(fs) != 2 {
		t.Fatal("uncertain metadata was merged")
	}
	for _, f := range fs {
		if f.Coverage != 1 || len(f.RelatedFamilies) != 1 || f.RelatedFamilies[0].Confidence != "POSSIBLE" {
			t.Fatalf("missing explicit relationship: %+v", f)
		}
	}
}

func TestUncertainSameSizeTargetDoesNotBecomeMissing(t *testing.T) {
	a, b, target := result(1, 1000, title), result(2, 1000, title), result(3, 1000, title)
	target.Parsed.AudioCodec = ""
	families := build([]models.Result{a, b, target}, request(), []models.IndexerProgress{{IndexerID: 3, Status: "completed"}})
	for _, f := range families {
		if f.TargetStatus.Candidate {
			t.Fatal("uncertain target metadata was treated as absence")
		}
	}
}
