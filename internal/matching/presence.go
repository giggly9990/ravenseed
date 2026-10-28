package matching

import (
	"math"
	"ravenseed/internal/models"
	"ravenseed/internal/releases"
	"strings"
)

// SameContent accepts punctuation variants and verified alternate titles while
// retaining contradictory IDs, and unverified contradictory years, as hard
// mismatches.
func SameContent(a, b models.ParsedRelease) bool {
	if a.MediaType == "" || a.MediaType != b.MediaType {
		return false
	}
	if a.ContentID != "" && b.ContentID != "" {
		// Matching verified IDs supersede title spelling and release-year drift.
		return a.ContentID == b.ContentID
	}
	if a.Year > 0 && b.Year > 0 && a.Year != b.Year {
		return false
	}
	return a.Title != "" && b.Title != "" && releases.TitleKey(a.Title) == releases.TitleKey(b.Title)
}

// ObservedEquivalent identifies an existing listing, without weakening family
// matching or claiming verified torrent contents. Exact positive bytes and all
// core release fields must agree. Exact bytes already tie the listings to one
// file, so a tag named on one side only (resolution, codec, audio, service,
// language, HDR, cut, correction) is a naming difference; tags named on both
// sides must still agree. Dual-audio annotations are ignored.
func ObservedEquivalent(a, b models.Result) bool {
	return a.Size > 0 && a.Size == b.Size && sameRelease(a.Parsed, b.Parsed)
}

// RoundedEquivalent finds a listing whose reported size is this release's size
// rounded for display, as trackers without exact byte sizes report it, and
// whose release fields agree. Rounding can hide a modified file, so it calls
// for review and never proves presence.
func RoundedEquivalent(a, b models.Result) bool {
	return a.Size > 0 && b.Size > 0 && a.Size != b.Size && roundedSize(a.Size, b.Size) && sameRelease(a.Parsed, b.Parsed)
}

// roundedSize reports whether reported equals exact shown in GiB, GB, MiB or
// MB with up to two decimal places.
func roundedSize(exact, reported int64) bool {
	for _, unit := range []float64{1 << 30, 1e9, 1 << 20, 1e6} {
		for _, places := range []float64{1, 10, 100} {
			shown := math.Round(float64(exact)/unit*places) / places
			if shown > 0 && math.Abs(shown*unit-float64(reported)) < 2 {
				return true
			}
		}
	}
	return false
}

func sameRelease(x, y models.ParsedRelease) bool {
	if !SameContent(x, y) {
		return false
	}
	for _, p := range []models.ParsedRelease{x, y} {
		if p.Group == "" || p.Source == "" || p.Source == "UNKNOWN" || p.Structure == "" || strings.Contains(p.Structure, "unknown") {
			return false
		}
	}
	// Names often omit these, SD names usually omit the resolution. A field
	// named on one side only is filled in; fields named on both must agree.
	for _, f := range []func(*models.ParsedRelease) *string{
		func(p *models.ParsedRelease) *string { return &p.Resolution },
		func(p *models.ParsedRelease) *string { return &p.VideoCodec },
		func(p *models.ParsedRelease) *string { return &p.BitDepth },
		func(p *models.ParsedRelease) *string { return &p.AudioCodec },
		func(p *models.ParsedRelease) *string { return &p.AudioChannels },
		func(p *models.ParsedRelease) *string { return &p.Service },
	} {
		a, b := f(&x), f(&y)
		if *a == "" {
			*a = *b
		} else if *b == "" {
			*b = *a
		}
	}
	// Tracker titles often drop language, HDR and cut tags the release name
	// carries; with identical bytes a tag on one side only is no difference.
	for _, f := range []func(*models.ParsedRelease) *[]string{
		func(p *models.ParsedRelease) *[]string { return &p.Language },
		func(p *models.ParsedRelease) *[]string { return &p.HDR },
		func(p *models.ParsedRelease) *[]string { return &p.Edition },
		func(p *models.ParsedRelease) *[]string { return &p.Cut },
	} {
		a, b := f(&x), f(&y)
		if len(*a) == 0 {
			*a = *b
		} else if len(*b) == 0 {
			*b = *a
		}
	}
	// A correction named on one side only, or another revision of the same
	// correction, is a naming difference: a repack changes the file's bytes.
	if x.Correction == "" || y.Correction == "" || x.Correction == y.Correction {
		x.Correction, x.Revision, x.Repack, x.Proper = "", 0, false, false
		y.Correction, y.Revision, y.Repack, y.Proper = "", 0, false, false
	}
	if x.ContentID == "" || y.ContentID == "" {
		x.ContentID, y.ContentID = "", ""
	}
	if x.Year == 0 || y.Year == 0 {
		x.Year, y.Year = 0, 0
	}
	x.AudioCodec, y.AudioCodec = presenceAudio(x.AudioCodec), presenceAudio(y.AudioCodec)
	return Fingerprint(x) == Fingerprint(y)
}

func presenceAudio(value string) string {
	parts := []string{}
	for _, part := range strings.Split(value, "+") {
		if part != "DUAL.AUDIO" {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, "+")
}
