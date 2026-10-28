package releases

import (
	"ravenseed/internal/models"
	"testing"
)

func TestBluRayDiscAndEncodeNames(t *testing.T) {
	for _, tc := range []struct{ title, want string }{
		{"Star Trek Beyond 2016 2160p UHD Blu-ray HDR HEVC TrueHD 7.1 Atmos-SharpHD", "FULL_DISC"},
		{"Star.Trek.Beyond.2016.2160p.UHD.BluRay.x265.HDR.TrueHD.7.1-SharpHD", "ENCODE"},
		{"Star.Trek.Beyond.2016.2160p.UHD.BluRay.REMUX.HEVC.TrueHD.7.1-SharpHD", "REMUX"},
		{"Example.2024.1080p.BluRay.AVC.DTS-HD.MA.5.1-GROUP", "FULL_DISC"},
		{"Example.2024.1080p.BluRay.ENCODE.AVC.DTS.5.1-GROUP", "ENCODE"},
		{"Example.2024.1080p.BluRay.AV1-GROUP", "ENCODE"},
		{"Example.2024.1080p.WEB-DL.HEVC-GROUP", "WEB_DL"},
	} {
		t.Run(tc.title, func(t *testing.T) {
			result := models.Result{Title: tc.title}
			Parse(&result, models.Media{Type: "movie"})
			if result.Parsed.ReleaseType != tc.want {
				t.Fatalf("got %s, want %s", result.Parsed.ReleaseType, tc.want)
			}
		})
	}
}

func TestTitlePunctuationKey(t *testing.T) {
	if TitleKey("FBI: International") != TitleKey("FBI.International") {
		t.Fatal("title punctuation differs")
	}
	if TitleKey("FBI") == TitleKey("FBI International") {
		t.Fatal("different series merged")
	}
	for _, pair := range [][2]string{
		{"Schindler's List", "Schindlers.List"},
		{"Schindler’s List", "Schindlers List"},
		{"Amélie", "Amelie"},
		{"Pokémon Detective Pikachu", "Pokemon.Detective.Pikachu"},
		{"Fast & Furious", "Fast.and.Furious"},
		{"Mr. & Mrs. Smith", "Mr.and.Mrs.Smith"},
	} {
		if TitleKey(pair[0]) != TitleKey(pair[1]) {
			t.Errorf("%q and %q keyed differently: %q vs %q", pair[0], pair[1], TitleKey(pair[0]), TitleKey(pair[1]))
		}
	}
	if TitleKey("Fast & Furious") == TitleKey("Fast Furious") {
		t.Fatal("ampersand dropped instead of normalized")
	}
}

func TestDailyEpisodeIsEpisodic(t *testing.T) {
	if !Episodic("The.Daily.Show.2024.10.01.1080p.WEB.h264-GRP") {
		t.Fatal("date-based episode not detected")
	}
	for _, name := range []string{"The.Matrix.1999.1080p.BluRay.x264-GRP", "Blade.Runner.2049.2017.2160p.UHD.BluRay.x265-GRP"} {
		if Episodic(name) {
			t.Fatalf("%s detected as episodic", name)
		}
	}
	r := models.Result{Title: "The.Daily.Show.2024.10.01.1080p.WEB.h264-GRP"}
	Parse(&r, models.Media{Type: "tv"})
	if r.Parsed.MediaType != "tv" || r.Parsed.Structure != "date:2024-10-01" {
		t.Fatalf("daily episode parsed as %+v", r.Parsed)
	}
}
