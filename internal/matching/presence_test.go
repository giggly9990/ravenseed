package matching

import (
	"math"
	"ravenseed/internal/models"
	"ravenseed/internal/releases"
	"testing"
)

func TestObservedEquivalentPresence(t *testing.T) {
	source := models.Result{Size: 28851404800, Title: "Vinland Saga S01 1080p NF WEB-DL DDP2 0 H 264-playWEB"}
	target := models.Result{Size: source.Size, Title: "Vinland Saga S01 1080p NF WEB-DL Dual-Audio DD+ 2.0 H.264-playWEB"}
	releases.Parse(&source, models.Media{Type: "tv"})
	releases.Parse(&target, models.Media{Type: "tv"})
	if !ObservedEquivalent(source, target) {
		t.Fatal("optional dual-audio annotation hid an existing listing")
	}
	if Fingerprint(source.Parsed) == Fingerprint(target.Parsed) {
		t.Fatal("presence handling must not erase audio annotations from family matching")
	}
	for _, tc := range []struct {
		name   string
		change func(*models.Result)
	}{
		{"different bytes", func(r *models.Result) { r.Size++ }},
		{"unknown bytes", func(r *models.Result) { r.Size = 0 }},
		{"audio codec", func(r *models.Result) { r.Parsed.AudioCodec = "FLAC" }},
		{"audio channels", func(r *models.Result) { r.Parsed.AudioChannels = "5.1" }},
		{"provider", func(r *models.Result) { r.Parsed.Service = "CR" }},
		{"group", func(r *models.Result) { r.Parsed.Group = "OTHER" }},
		{"unknown group", func(r *models.Result) { r.Parsed.Group = "" }},
		{"season", func(r *models.Result) { r.Parsed.Structure = "seasons:2"; r.Parsed.Season = 2 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := target
			tc.change(&changed)
			if ObservedEquivalent(source, changed) {
				t.Fatal("different or incomplete release treated as equivalent")
			}
		})
	}
}

func TestPresenceTitlePunctuationAndIdentity(t *testing.T) {
	a := models.Result{Size: 54645880080, Title: "FBI.International.S04.1080p.AMZN.WEB-DL.DDP5.1.H.264-NTb"}
	b := models.Result{Size: 54645880080, Title: "FBI: International S04 1080p AMZN WEB-DL DD+ 5.1 H.264-NTb"}
	releases.Parse(&a, models.Media{Type: "tv"})
	releases.Parse(&b, models.Media{Type: "tv"})
	if !ObservedEquivalent(a, b) {
		t.Fatal("punctuation hid equivalent TV release")
	}
	a.Parsed.ContentID, b.Parsed.ContentID = "tv:tmdb:1", "tv:tmdb:2"
	if ObservedEquivalent(a, b) {
		t.Fatal("conflicting media IDs accepted")
	}
}

func TestVerifiedIdentityOverridesYearDrift(t *testing.T) {
	a := models.Result{Size: 9000, Title: "Parasite.2019.1080p.BluRay.x264-GRP", IdentityVerified: true}
	b := models.Result{Size: 9000, Title: "Parasite 2020 1080p BluRay x264-GRP", IdentityVerified: true}
	media := models.Media{Type: "movie", IMDbID: "tt6751668"}
	releases.Parse(&a, media)
	releases.Parse(&b, media)
	if !SameContent(a.Parsed, b.Parsed) {
		t.Fatal("verified identical IDs rejected over a release-year difference")
	}
	if !ObservedEquivalent(a, b) {
		t.Fatal("verified identical IDs hid an existing listing")
	}
	if c, e := compare(a, b); c == "REJECTED" {
		t.Fatalf("verified identical IDs rejected: %v", e)
	}
	a.Parsed.ContentID, b.Parsed.ContentID = "", ""
	if SameContent(a.Parsed, b.Parsed) {
		t.Fatal("unverified year conflict accepted")
	}
}

func TestPresenceToleratesOneSidedMissingResolution(t *testing.T) {
	source := models.Result{Size: 11417325401, Title: "Scrubs.S06.DSNP.WEB-DL.AAC2.0.H.264-playWEB"}
	target := models.Result{Size: source.Size, Title: "Scrubs S06 1080p DSNP WEB-DL AAC 2.0 H.264-playWEB"}
	releases.Parse(&source, models.Media{Type: "tv"})
	releases.Parse(&target, models.Media{Type: "tv"})
	if !ObservedEquivalent(source, target) || !ObservedEquivalent(target, source) {
		t.Fatal("exact bytes and matching release fields hidden by an unnamed resolution")
	}
	other := target
	other.Size++
	if ObservedEquivalent(source, other) {
		t.Fatal("different bytes accepted")
	}
}

func TestRoundedEquivalent(t *testing.T) {
	const exact = int64(11417325401) // 10.633 GiB, 11.417 GB
	source := models.Result{Size: exact, Title: "Scrubs.S06.1080p.DSNP.WEB-DL.AAC2.0.H.264-playWEB"}
	releases.Parse(&source, models.Media{Type: "tv"})
	for name, reported := range map[string]int64{
		"GiB, two places": int64(math.Round(10.63 * (1 << 30))),
		"GiB, one place":  int64(math.Round(10.6 * (1 << 30))),
		"GB, two places":  int64(math.Round(11.42 * 1e9)),
	} {
		target := source
		target.Size = reported
		if !RoundedEquivalent(source, target) {
			t.Errorf("%s: rounded listing %d not recognized", name, reported)
		}
		if ObservedEquivalent(source, target) {
			t.Errorf("%s: rounded size counted as exact presence", name)
		}
	}
	for name, reported := range map[string]int64{
		"next display value":  int64(math.Round(10.64 * (1 << 30))),
		"unrounded neighbour": exact + 4096,
		"identical":           exact,
	} {
		target := source
		target.Size = reported
		if RoundedEquivalent(source, target) {
			t.Errorf("%s: accepted as a rounded listing", name)
		}
	}
	target := source
	target.Size = int64(math.Round(10.63 * (1 << 30)))
	target.Parsed.Group = "OTHER"
	if RoundedEquivalent(source, target) {
		t.Fatal("different group accepted on rounded size")
	}
}

func TestPresenceForSDNamesWithoutResolution(t *testing.T) {
	for _, pair := range [][2]string{
		{"Scrubs.S01.DVDRip.x264-GRP", "Scrubs S01 DVDRip x264-GRP"},
		{"Scrubs.S01.DVDRip.x264-GRP", "Scrubs S01 480p DVDRip AC3 2.0 x264-GRP"},
		{"Show.S01E01.SDTV.XviD-GRP", "Show S01E01 SDTV XviD-GRP"},
		{"Show.S01E01.WEB-DL-GRP", "Show S01E01 720p WEB-DL DDP5.1 H.264-GRP"},
	} {
		a := models.Result{Size: 734003200, Title: pair[0]}
		b := models.Result{Size: a.Size, Title: pair[1]}
		releases.Parse(&a, models.Media{Type: "tv"})
		releases.Parse(&b, models.Media{Type: "tv"})
		if !ObservedEquivalent(a, b) {
			t.Errorf("%q vs %q: exact bytes with unnamed fields not present", pair[0], pair[1])
		}
	}
	for _, pair := range [][2]string{
		{"Scrubs.S01.DVDRip.x264-GRP", "Scrubs S01 DVDRip XviD-GRP"},
		{"Scrubs.S01.DVDRip.x264-GRP", "Scrubs S01 DVDRip x264-OTHER"},
		{"Scrubs.S01.DVDRip.x264-GRP", "Scrubs S01 SDTV x264-GRP"},
		{"Scrubs.S01.DVDRip.x264-GRP", "Scrubs S02 DVDRip x264-GRP"},
		{"Scrubs.S01.DVDRip.x264-GRP", "Scrubs S01 DVDRip x264"},
	} {
		a := models.Result{Size: 734003200, Title: pair[0]}
		b := models.Result{Size: a.Size, Title: pair[1]}
		releases.Parse(&a, models.Media{Type: "tv"})
		releases.Parse(&b, models.Media{Type: "tv"})
		if ObservedEquivalent(a, b) {
			t.Errorf("%q vs %q: contradicting or missing core field accepted", pair[0], pair[1])
		}
	}
}

func TestExactBytesOutweighCorrectionLabels(t *testing.T) {
	tv := models.Media{Type: "tv"}
	source := models.Result{Size: 11790977098, Title: "Cyberpunk.Edgerunners.S01.REPACK.1080p.NF.WEB-DL.DUAL.DDP5.1.H.264-FLUX"}
	target := models.Result{Size: source.Size, Title: "Cyberpunk: Edgerunners S01 REPACK3 1080p NF WEB-DL Dual-Audio DD+ 5.1 H.264-FLUX"}
	releases.Parse(&source, tv)
	releases.Parse(&target, tv)
	if !ObservedEquivalent(source, target) {
		t.Fatalf("one file named REPACK and REPACK3 not equivalent: %+v vs %+v", source.Parsed, target.Parsed)
	}
	untagged := models.Result{Size: source.Size, Title: "Cyberpunk Edgerunners S01 1080p NF WEB-DL DUAL DDP5.1 H.264-FLUX"}
	releases.Parse(&untagged, tv)
	if !ObservedEquivalent(source, untagged) {
		t.Fatal("a correction named on one side only hid the same file")
	}
	proper := models.Result{Size: source.Size, Title: "Cyberpunk Edgerunners S01 PROPER 1080p NF WEB-DL DUAL DDP5.1 H.264-FLUX"}
	releases.Parse(&proper, tv)
	if ObservedEquivalent(source, proper) {
		t.Fatal("REPACK and PROPER labels treated as one release")
	}
	other := target
	other.Size++
	if ObservedEquivalent(source, other) {
		t.Fatal("different bytes accepted")
	}
}

func TestExactBytesOutweighOneSidedTags(t *testing.T) {
	tv := models.Media{Type: "tv"}
	source := models.Result{Size: 8318738147, Title: "Its.Always.Sunny.in.Philadelphia.S01.NORDiC.1080p.WEB.H264-EGEN"}
	target := models.Result{Size: source.Size, Title: "It's Always Sunny in Philadelphia S01 1080p WEB-DL DD 2.0 H.264-EGEN"}
	releases.Parse(&source, tv)
	releases.Parse(&target, tv)
	if !ObservedEquivalent(source, target) {
		t.Fatalf("a language tag on one side hid the same file: %+v vs %+v", source.Parsed, target.Parsed)
	}
	for name, change := range map[string]func(*models.ParsedRelease){
		"hdr":     func(p *models.ParsedRelease) { p.HDR = []string{"HDR10"} },
		"edition": func(p *models.ParsedRelease) { p.Edition = []string{"EXTENDED"} },
	} {
		tagged := target
		change(&tagged.Parsed)
		if !ObservedEquivalent(source, tagged) {
			t.Errorf("%s named on one side hid the same file", name)
		}
	}
	for name, change := range map[string]func(a, b *models.ParsedRelease){
		"language": func(a, b *models.ParsedRelease) { a.Language, b.Language = []string{"NORDIC"}, []string{"GERMAN"} },
		"hdr":      func(a, b *models.ParsedRelease) { a.HDR, b.HDR = []string{"HDR10"}, []string{"DV"} },
		"edition": func(a, b *models.ParsedRelease) {
			a.Edition, b.Edition = []string{"EXTENDED"}, []string{"THEATRICAL"}
		},
	} {
		a, b := source, target
		change(&a.Parsed, &b.Parsed)
		if ObservedEquivalent(a, b) {
			t.Errorf("conflicting %s accepted", name)
		}
	}
}
