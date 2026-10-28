package clustering

import (
	"fmt"
	"slices"
	"testing"

	"ravenseed/internal/models"
	"ravenseed/internal/releases"
)

// Sizes within tolerance mark a modified copy (stripped metadata) of one
// release: they stay one STRONG family, and STRONG never makes a candidate.
func TestWithinToleranceStaysStrongFamily(t *testing.T) {
	progress := []models.IndexerProgress{{IndexerID: 3, Status: "completed"}}
	rules := releases.DefaultRules()
	rules.SizeTolerancePercent = 1
	a, b := result(1, 10000000, title), result(2, 10050000, title)
	fs := BuildWithRules([]models.Result{a, b}, request(), progress, rules)
	if len(fs) != 1 || fs[0].Confidence != "STRONG" || len(fs[0].Variants) != 2 {
		t.Fatalf("within-tolerance copies split or lost STRONG: %+v", fs)
	}
	if fs[0].TargetStatus.Candidate || slices.ContainsFunc(fs[0].Variants, func(v models.SizeVariant) bool { return v.TargetStatus.Candidate }) {
		t.Fatalf("STRONG family became a target candidate: %+v", fs[0].TargetStatus)
	}
	// The member on the reference's own indexer needs another indexer; the
	// other member compares across indexers and rates STRONG.
	if !slices.ContainsFunc(fs[0].Results, func(r models.Result) bool { return r.Confidence == "STRONG" }) {
		t.Fatalf("no member rated STRONG: %+v", fs[0].Results)
	}
	far := result(2, 20000000, title)
	if fs := BuildWithRules([]models.Result{a, far}, request(), progress, rules); len(fs) != 1 || fs[0].Confidence != "POSSIBLE" {
		t.Fatalf("size outside tolerance rated: %+v", fs)
	}
}

func TestSameFileMergeIsNotTransitive(t *testing.T) {
	tv := models.Media{Type: "tv", TMDbID: 1399}
	parse := func(id int, name string) models.Result {
		r := models.Result{ID: fmt.Sprint(id), IndexerID: id, Title: name, Size: 5000000, IdentityVerified: true}
		releases.Parse(&r, tv)
		return r
	}
	amzn := parse(1, "Show.S01E01.1080p.AMZN.WEB-DL.DDP5.1.H.264-GRP")
	plain := parse(2, "Show.S01E01.1080p.WEB-DL.DDP5.1.H.264-GRP")
	nf := parse(3, "Show.S01E01.1080p.NF.WEB-DL.DDP5.1.H.264-GRP")
	for _, order := range [][]models.Result{{amzn, plain, nf}, {nf, plain, amzn}, {plain, nf, amzn}} {
		fs := build(order, request(), nil)
		for _, f := range fs {
			services := []string{}
			for _, r := range f.Results {
				services = append(services, r.Parsed.Service)
			}
			if slices.Contains(services, "AMZN") && slices.Contains(services, "NF") {
				t.Fatalf("AMZN and NF titles merged through a service-less title: %v", services)
			}
		}
		if len(fs) != 2 {
			t.Fatalf("service-less title not merged with one side: %d families", len(fs))
		}
	}
}

func TestUnknownSizeVariantsAreDeterministic(t *testing.T) {
	a, b, c := result(1, 0, title), result(2, 0, title), result(1, 10000000, title)
	a.ID, b.ID, c.ID = "b", "a", "c"
	var first models.Family
	for i, order := range [][]models.Result{{a, b, c}, {c, b, a}, {b, c, a}, {a, c, b}} {
		fs := build(order, request(), nil)
		if len(fs) != 1 {
			t.Fatalf("families: %+v", fs)
		}
		if i == 0 {
			first = fs[0]
			continue
		}
		for j := range first.Variants {
			if first.Variants[j].ResultIDs[0] != fs[0].Variants[j].ResultIDs[0] {
				t.Fatalf("variant order depends on input order: %v vs %v", first.Variants, fs[0].Variants)
			}
		}
	}
}
