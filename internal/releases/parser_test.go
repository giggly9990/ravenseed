package releases

import (
	"ravenseed/internal/models"
	"slices"
	"strings"
	"testing"
)

func TestNormalization(t *testing.T) {
	tests := []struct{ name, title, kind, source, codec, group string }{
		{"remux", "The.Matrix.1999.2160p.UHD.BluRay.REMUX.HEVC.TrueHD.7.1-FraMeSToR", "REMUX", "UHD_BLURAY", "HEVC", "FRAMESTOR"},
		{"encode", "The.Matrix.1999.1080p.BluRay.x264.DTS-GROUP", "ENCODE", "BLURAY", "AVC", "GROUP"},
		{"webdl", "The.Matrix.1999.1080p.WEB-DL.DDP5.1.H.264-NTb", "WEB_DL", "WEB_DL", "AVC", "NTB"},
		{"webrip", "The.Matrix.1999.1080p.WEBRip.x264-GROUP", "WEBRIP", "WEB", "AVC", "GROUP"},
		{"disc", "The.Matrix.1999.2160p.UHD.BluRay.COMPLETE.HEVC-GROUP", "FULL_DISC", "UHD_BLURAY", "HEVC", "GROUP"},
		{"hdtv", "Example.Show.S01E01.720p.HDTV.x264-GROUP", "HDTV", "HDTV", "AVC", "GROUP"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := models.Result{Title: tt.title}
			Parse(&r, models.Media{Type: "movie"})
			p := r.Parsed
			if p.ReleaseType != tt.kind || p.Source != tt.source || p.VideoCodec != tt.codec || p.Group != tt.group {
				t.Fatalf("unexpected parse: %+v", p)
			}
			if len(r.ParserOutput) == 0 {
				t.Fatal("missing original parser output")
			}
		})
	}
}
func TestFlagsAndTVStructure(t *testing.T) {
	for _, title := range []string{"Example.Show.S01.1080p.WEB-DL.H.264-NTb", "Example.Show.S01E01.1080p.WEB-DL.H.264-NTb", "Example.Show.S01E01E02.1080p.WEB-DL.H.264-NTb"} {
		r := models.Result{Title: title}
		Parse(&r, models.Media{Type: "tv"})
		if r.Parsed.Season != 1 || r.Parsed.Structure == "unknown" {
			t.Fatalf("TV metadata missing: %+v", r.Parsed)
		}
	}
	r := models.Result{Title: "The.Matrix.1999.Directors.Cut.2160p.UHD.BluRay.REMUX.REPACK.PROPER.HEVC.DV.HDR10+.TrueHD.Atmos.7.1-GROUP"}
	Parse(&r, models.Media{Type: "movie"})
	p := r.Parsed
	if !p.Repack || !p.Proper || !slices.Contains(p.HDR, "DV") || !slices.Contains(p.HDR, "HDR10+") {
		t.Fatalf("flags missing: %+v", p)
	}
	if len(p.Cut)+len(p.Edition) == 0 {
		t.Fatal("edition missing")
	}
}
func TestAnimeGroupPreserved(t *testing.T) {
	r := models.Result{Title: "[SubsPlease] Example Show - 01 (1080p) [AB123456].mkv"}
	Parse(&r, models.Media{Type: "tv"})
	if r.Parsed.Group != "SUBSPLEASE" {
		t.Fatalf("anime group: %q", r.Parsed.Group)
	}
}

func TestPQ10PreservedForPolicyEvaluation(t *testing.T) {
	r := models.Result{Title: "Example.Movie.2024.2160p.PQ10.WEB-DL.H.265-GROUP"}
	Parse(&r, models.Media{Type: "movie"})
	if !slices.Contains(r.Parsed.HDR, "PQ10") {
		t.Fatalf("PQ10 was not parsed: %+v", r.Parsed)
	}
	if strings.Contains(strings.ToUpper(r.Parsed.Title), "PQ10") {
		t.Fatalf("PQ10 was absorbed into title identity: %+v", r.Parsed)
	}
}
