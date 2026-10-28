package releases

import (
	"slices"
	"strings"
	"testing"

	"ravenseed/internal/models"
)

func TestTitleDropsStructureLeftovers(t *testing.T) {
	for _, tt := range []struct{ title, want string }{
		{"Show.S01E01v2.1080p.WEB-DL.H.264-GRP", "Show"},
		{"Show.Seasons.1-3.1080p.WEB-DL.H.264-GRP", "Show"},
		{"Show.S01E01.1080p.WEB-DL.H.264-GRP", "Show"},
		{"The.Office.US.S01E01.720p.HDTV.x264-GRP", "The Office"},
	} {
		r := models.Result{Title: tt.title}
		Parse(&r, models.Media{Type: "tv"})
		if r.Parsed.Title != tt.want {
			t.Errorf("%s: title %q, want %q", tt.title, r.Parsed.Title, tt.want)
		}
	}
}

func TestStructureEdgeCases(t *testing.T) {
	for _, tt := range []struct{ title, media, mediaType, structure string }{
		{"[Group] Show S2 - 05 (1080p) [ABCDEF12].mkv", "tv", "tv", "episodes:s2:e5"},
		{"Show - S2 - 03 (1080p)", "tv", "tv", "episodes:s2:e3"},
		{"Show.S01-S03.1080p.WEB-DL.H.264-GRP", "tv", "tv", "seasons:1,2,3"},
		{"Show S1 - S3 1080p WEB-DL H.264-GRP", "tv", "tv", "seasons:1,2,3"},
		{"Show.S01E01-S01E03.1080p.WEB-DL.H.264-GRP", "tv", "tv", "episodes:s1:e1,2,3"},
		{"Show.S01E01-S02E03.1080p.WEB-DL.H.264-GRP", "tv", "tv", "unknown:multi_season_episodes"},
		{"Show S01E01 - E03 1080p WEB-DL H.264-GRP", "tv", "tv", "episodes:s1:e1,2,3"},
		{"Show.1x02.720p.HDTV.x264-GRP", "tv", "tv", "episodes:s1:e2"},
		{"Movie.1999.DVDRip.640x360-GRP", "movie", "movie", "movie"},
		{"Movie.1999.1920x1080.WEB-DL.H.264-GRP", "movie", "movie", "movie"},
	} {
		r := models.Result{Title: tt.title}
		Parse(&r, models.Media{Type: tt.media})
		if r.Parsed.MediaType != tt.mediaType || r.Parsed.Structure != tt.structure {
			t.Errorf("%s: %s %s, want %s %s", tt.title, r.Parsed.MediaType, r.Parsed.Structure, tt.mediaType, tt.structure)
		}
	}
}

func TestCorrectionAndHybridTags(t *testing.T) {
	for _, tt := range []struct {
		title          string
		correction     string
		revision       int
		repack, hybrid bool
		proper         bool
	}{
		{title: "Movie.1999.REPACK.5.1.1080p.BluRay.x264-GRP", correction: "REPACK", revision: 1, repack: true},
		{title: "Movie.1999.REPACK.2.1080p.BluRay.x264-GRP", correction: "REPACK", revision: 2, repack: true},
		{title: "Movie.1999.REPACK2.DDP.5.1.1080p.WEB-DL.H.264-GRP", correction: "REPACK", revision: 2, repack: true},
		{title: "Movie.Repack.2013.1080p.BluRay.x264-GRP", correction: "REPACK", revision: 1, repack: true},
		{title: "Hybrid.2013.1080p.BluRay.x264-GRP", revision: 1},
		{title: "Proper.Title.2013.1080p.BluRay.x264-GRP", revision: 1},
		{title: "Movie.2013.Hybrid.1080p.BluRay.x264-GRP", revision: 1, hybrid: true},
		{title: "Movie.PROPER.1080p.BluRay.x264-GRP", correction: "PROPER", revision: 1, proper: true},
		{title: "Show.S01E01.PROPER.720p.HDTV.x264-GRP", correction: "PROPER", revision: 1, proper: true},
	} {
		r := models.Result{Title: tt.title}
		Parse(&r, models.Media{})
		p := r.Parsed
		if p.Correction != tt.correction || p.Revision != tt.revision || p.Repack != tt.repack || p.Proper != tt.proper || p.Hybrid != tt.hybrid {
			t.Errorf("%s: correction=%q revision=%d repack=%v proper=%v hybrid=%v", tt.title, p.Correction, p.Revision, p.Repack, p.Proper, p.Hybrid)
		}
	}
}

func TestReleaseTypeEdgeCases(t *testing.T) {
	for _, tt := range []struct{ title, kind, source string }{
		{"Show.S01.COMPLETE.1080p.BluRay.H.264-GRP", "ENCODE", "BLURAY"},
		{"Show.S01.1080p.BluRay.10bit.HEVC.AAC-GRP", "ENCODE", "BLURAY"},
		{"Movie.2010.COMPLETE.BLURAY-GRP", "FULL_DISC", "BLURAY"},
		{"Movie.2010.DVD.ISO-GRP", "OTHER", "DVD"},
		{"Movie.2010.1080p.BD50.ISO-GRP", "FULL_DISC", "BLURAY"},
	} {
		r := models.Result{Title: tt.title}
		Parse(&r, models.Media{})
		if r.Parsed.ReleaseType != tt.kind || r.Parsed.Source != tt.source {
			t.Errorf("%s: %s %s, want %s %s", tt.title, r.Parsed.ReleaseType, r.Parsed.Source, tt.kind, tt.source)
		}
	}
}

func TestGroupExtraction(t *testing.T) {
	for _, tt := range []struct{ title, group string }{
		{"Movie.2010.1080p.BluRay.x264-GRP-Sample", "GRP"},
		{"Movie.2010.1080p.WEB-DL.H.264-NTb-AsRequested", "NTB"},
		{"Movie.2010.1080p.BluRay.x264-D-Z0N3", "D-Z0N3"},
		{"Movie.2010.1080p.BluRay.x264-E.N.D", "E.N.D"},
		{"[Judas] Show - 01-12 (1080p) (Batch)", "JUDAS"},
		{"Movie.2010.1080p.BluRay.x264-[GRP]", "GRP"},
		{"Movie.2010.1080p.BluRay.DTS-X.7.1.x264-GRP", "GRP"},
		{"Movie.2010.1080p.WEB-DL.DD-5.1", ""},
	} {
		r := models.Result{Title: tt.title}
		Parse(&r, models.Media{})
		if r.Parsed.Group != tt.group {
			t.Errorf("%s: group %q, want %q", tt.title, r.Parsed.Group, tt.group)
		}
	}
}

func TestServiceAndResolutionTail(t *testing.T) {
	for _, tt := range []struct{ title, service string }{
		{"Movie.2010.1080p.MAX.WEB-DL.H.264-GRP", "HMAX"},
		{"Movie.2010.1440p.AMAZON.WEB-DL.H.264-GRP", "AMZN"},
		{"Movie.2010.1080p.WEB-DL.H.264-MAX", ""},
	} {
		r := models.Result{Title: tt.title}
		Parse(&r, models.Media{Type: "movie"})
		if r.Parsed.Service != tt.service {
			t.Errorf("%s: service %q, want %q", tt.title, r.Parsed.Service, tt.service)
		}
	}
}

func TestMultiWordIgnoredTag(t *testing.T) {
	rules, err := NormalizeRules(&models.MatchingRules{IgnoredTags: []string{"Custom Tag"}})
	if err != nil {
		t.Fatal(err)
	}
	r := models.Result{Title: "Movie.2010.1080p.Custom.Tag.WEB-DL.H.264-GRP"}
	ParseWithRules(&r, models.Media{Type: "movie"}, rules)
	if !slices.Contains(r.NormalizationEvidence, "ignored_cosmetic_tag:CUSTOM TAG") {
		t.Fatalf("multi-word tag not removed: %v", r.NormalizationEvidence)
	}
}

func TestParseBoundsHostileTitles(t *testing.T) {
	r := models.Result{Title: "Show." + strings.Repeat("S01E01-E500.", 200) + "1080p-GRP"}
	Parse(&r, models.Media{Type: "tv"})
	if len(r.Parsed.Episodes) > maxEpisodes {
		t.Fatalf("episode set not bounded: %d", len(r.Parsed.Episodes))
	}
	r = models.Result{Title: "Show.S01E01-E09999.1080p-GRP"}
	Parse(&r, models.Media{Type: "tv"})
	if len(r.Parsed.Episodes) > maxEpisodes {
		t.Fatalf("episode range not bounded: %d", len(r.Parsed.Episodes))
	}
}

func TestNormalizeIMDbID(t *testing.T) {
	for in, want := range map[string]string{"tt0133093": "tt0133093", "TT0133093": "tt0133093", "0133093": "tt0133093", "133093": "tt0133093", "tt10872600": "tt10872600", "tt0": "", "tt-5": "", "tt+5": "", "": "", "tt": "", "tt12345678901": "", "nm0000206x": ""} {
		if got := NormalizeIMDbID(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}
