package ranking

import (
	"testing"
	"time"

	"ravenseed/internal/models"
)

func family(id string, change func(*models.Family)) models.Family {
	f := models.Family{ID: id, Confidence: "EXACT", Variants: []models.SizeVariant{{Size: 100}}}
	change(&f)
	return f
}

func TestSortKeys(t *testing.T) {
	now := time.Now()
	for _, tt := range []struct {
		by    string
		first models.Family
		other models.Family
	}{
		{"quality", family("a", func(f *models.Family) { f.Canonical.ReleaseType, f.Canonical.Resolution = "REMUX", "2160p" }), family("b", func(f *models.Family) {
			f.Canonical.ReleaseType, f.Canonical.Resolution = "ENCODE", "2160p"
			f.Coverage = 9
		})},
		{"seeders", family("b", func(f *models.Family) { f.Seeders = 10 }), family("a", func(f *models.Family) { f.Seeders = 1 })},
		{"size", family("b", func(f *models.Family) { f.Variants[0].Size = 200 }), family("a", func(f *models.Family) {})},
		{"newest", family("b", func(f *models.Family) { f.Newest = now }), family("a", func(f *models.Family) { f.Newest = now.Add(-time.Hour) })},
		{"target", family("b", func(f *models.Family) { f.TargetStatus.Candidate = true; f.Confidence = "POSSIBLE" }), family("a", func(f *models.Family) { f.Coverage = 5 })},
		// coverage is the default order: confidence first, then coverage.
		{"coverage", family("b", func(f *models.Family) { f.Coverage = 2 }), family("a", func(f *models.Family) { f.Coverage = 9; f.Confidence = "POSSIBLE" })},
		{"coverage", family("b", func(f *models.Family) { f.Coverage = 3 }), family("a", func(f *models.Family) { f.Coverage = 2 })},
	} {
		fs := []models.Family{tt.other, tt.first}
		Sort(fs, tt.by)
		if fs[0].ID != tt.first.ID {
			t.Errorf("%s: %s sorted before %s", tt.by, fs[0].ID, tt.first.ID)
		}
	}
}

func TestSortTiesFallBackToID(t *testing.T) {
	fs := []models.Family{family("b", func(*models.Family) {}), family("a", func(*models.Family) {})}
	for _, by := range []string{"quality", "seeders", "size", "newest", "target", "coverage", ""} {
		Sort(fs, by)
		if fs[0].ID != "a" {
			t.Errorf("%s: tie not broken by ID", by)
		}
	}
}

func TestQuality(t *testing.T) {
	remux := models.Family{Canonical: models.ParsedRelease{ReleaseType: "REMUX", Resolution: "1080p"}}
	encode := models.Family{Canonical: models.ParsedRelease{ReleaseType: "ENCODE", Resolution: "2160p"}}
	if quality(remux) <= quality(encode) {
		t.Fatal("release class does not outrank resolution")
	}
}
