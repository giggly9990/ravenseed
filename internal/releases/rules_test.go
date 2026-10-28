package releases

import (
	"ravenseed/internal/models"
	"testing"
)

func TestRuleValidation(t *testing.T) {
	for _, aliases := range []map[string]string{{"A": "A"}, {"A": "B", "B": "A"}, {"A": "B", "a": "C"}, {"*": "GROUP"}} {
		r := DefaultRules()
		r.GroupAliases = aliases
		if _, err := NormalizeRules(&r); err == nil {
			t.Fatalf("accepted ambiguous aliases: %v", aliases)
		}
	}
	for _, tag := range []string{"REMUX", "HDR10", "CUSTOM HDR", "S01E01", "1080p", "GROUP.*", "PROPER", "HYBRID", "NF"} {
		r := DefaultRules()
		r.IgnoredTags = []string{tag}
		if _, err := NormalizeRules(&r); err == nil {
			t.Fatalf("accepted metadata ignore: %s", tag)
		}
	}
	for _, tolerance := range []float64{-1, 1.1} {
		r := DefaultRules()
		r.SizeTolerancePercent = tolerance
		if _, err := NormalizeRules(&r); err == nil {
			t.Fatal("invalid tolerance accepted")
		}
	}
	r := DefaultRules()
	r.GroupAliases = map[string]string{"old": "middle", "middle": "new"}
	got, err := NormalizeRules(&r)
	if err != nil || got.GroupAliases["OLD"] != "NEW" {
		t.Fatalf("alias chain not canonicalized: %+v %v", got, err)
	}
	r.EpisodeMappings = []models.EpisodeMapping{{TMDbID: 10, Absolute: 1, Season: 1, Episode: 1}, {TMDbID: 10, Absolute: 2, Season: 1, Episode: 1}}
	if _, err := NormalizeRules(&r); err == nil {
		t.Fatal("ambiguous episode mapping accepted")
	}
}

func TestSeasonEpisodeScope(t *testing.T) {
	r := models.Result{Title: "Example.Show.S00-S02.1080p.WEB-DL.H.264-GROUP"}
	Parse(&r, models.Media{Type: "tv"})
	season, episode := 2, 1
	if !InScope(r.Parsed, models.Media{Type: "tv", Season: &season}) {
		t.Fatal("multi-season pack lost requested season")
	}
	if InScope(r.Parsed, models.Media{Type: "tv", Season: &season, Episode: &episode}) {
		t.Fatal("pack treated as individual episode")
	}
	r.Title = "Example.Show.S02E01-E03.1080p.WEB-DL.H.264-GROUP"
	Parse(&r, models.Media{Type: "tv"})
	episode = 2
	if !InScope(r.Parsed, models.Media{Type: "tv", Season: &season, Episode: &episode}) {
		t.Fatalf("episode range lost requested episode: %+v", r.Parsed)
	}
}

func TestInScopeOrPack(t *testing.T) {
	season, episode := 1, 3
	m := models.Media{Type: "tv", Season: &season, Episode: &episode}
	for title, want := range map[string]bool{
		"Show.S01.1080p.WEB-DL.H.264-GRP":     true,
		"Show.S01-S03.1080p.WEB-DL.H.264-GRP": true,
		"Show.S01E03.1080p.WEB-DL.H.264-GRP":  true,
		"Show.S01E04.1080p.WEB-DL.H.264-GRP":  false,
		"Show.S02.1080p.WEB-DL.H.264-GRP":     false,
	} {
		r := models.Result{Title: title}
		Parse(&r, models.Media{Type: "tv"})
		if got := InScopeOrPack(r.Parsed, m); got != want {
			t.Errorf("%s: got %v want %v (%s)", title, got, want, r.Parsed.Structure)
		}
	}
	pack := models.Result{Title: "Show.S01.1080p.WEB-DL.H.264-GRP"}
	Parse(&pack, models.Media{Type: "tv"})
	whole := models.Media{Type: "tv", Season: &season}
	if !InScopeOrPack(pack.Parsed, whole) {
		t.Fatal("season scope lost its pack")
	}
}
