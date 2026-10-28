package releases

import (
	"ravenseed/internal/models"
	"slices"
	"testing"
)

func TestParsedResolutionsAreListed(t *testing.T) {
	for _, title := range []string{
		"Movie.2010.480p.DVDRip.x264-GRP", "Movie.2010.576p.BluRay.x264-GRP", "Movie.2010.720p.BluRay.x264-GRP",
		"Show.S01E01.1080i.HDTV.H.264-GRP", "Movie.2010.1080p.BluRay.x264-GRP", "Movie.2010.1440p.AMZN.WEB-DL.H.264-GRP",
		"Movie.2010.2160p.UHD.BluRay.x265-GRP", "Movie.2010.4K.WEB-DL.H.265-GRP", "Movie.2010.4320p.WEB-DL.H.265-GRP",
	} {
		r := models.Result{Title: title}
		Parse(&r, models.Media{Type: "movie"})
		if !slices.Contains(Resolutions, r.Parsed.Resolution) {
			t.Errorf("%s: resolution %q is not in Resolutions", title, r.Parsed.Resolution)
		}
	}
}

func TestClassRank(t *testing.T) {
	if ClassRank("FULL_DISC") != 7 || ClassRank("REMUX") != 6 || ClassRank("WEBRIP") != 3 || ClassRank("OTHER") != 1 || ClassRank("UNKNOWN") != 0 {
		t.Fatal("unexpected release class order")
	}
	if !resolutionTag.MatchString("Movie.1080i.HDTV") || !resolutionTag.MatchString("Movie.576i.DVD") || resolutionTag.MatchString("Movie.1081p") {
		t.Fatal("unexpected resolution tag match")
	}
}
