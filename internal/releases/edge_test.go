package releases

import (
	"ravenseed/internal/models"
	"testing"
)

func TestRevisionAndStructureEdgeCases(t *testing.T) {
	for _, tt := range []struct {
		title    string
		revision int
	}{
		{"Movie.1999.REPACK10.1080p.BluRay.x264-GROUP", 10},
		{"Movie.1999.REPACK0.1080p.BluRay.x264-GROUP", -1},
		{"Movie.1999.REPACK999999999999999999999.1080p.BluRay.x264-GROUP", -1},
		{"[SubsPlease] Example Show - 01v0 (1080p WEB-DL HEVC)", -1},
	} {
		r := models.Result{Title: tt.title}
		Parse(&r, models.Media{Type: "tv"})
		if r.Parsed.Revision != tt.revision {
			t.Errorf("revision %q = %d, want %d", tt.title, r.Parsed.Revision, tt.revision)
		}
	}
	for _, title := range []string{"Show.S01E04-E01.1080p.WEB-DL.H.264-GROUP", "Show.2026.99.99.1080p.WEB-DL.H.264-GROUP", "Show.S01E01.S02E01.1080p.WEB-DL.H.264-GROUP"} {
		r := models.Result{Title: title}
		Parse(&r, models.Media{Type: "tv"})
		if len(r.Parsed.Structure) < 7 || r.Parsed.Structure[:7] != "unknown" {
			t.Errorf("ambiguous structure accepted: %+v", r.Parsed)
		}
	}
}
