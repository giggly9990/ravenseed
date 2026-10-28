package releases

import (
	"ravenseed/internal/models"
	"testing"
)

func TestReissueUsesExistingCorrectionParser(t *testing.T) {
	for _, title := range []string{"Example.2024.1080p.WEB-DL.REISSUE.H.264-GROUP", "Example.2024.1080p.WEB-DL.RE-ISSUE.H.264-GROUP"} {
		r := models.Result{Title: title}
		Parse(&r, models.Media{Type: "movie"})
		if r.Parsed.Correction != "REISSUE" {
			t.Fatalf("%s: correction=%q", title, r.Parsed.Correction)
		}
	}
}
