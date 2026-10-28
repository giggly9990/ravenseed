package matching

import (
	"encoding/json"
	"math"
	"strings"

	"ravenseed/internal/models"
	"ravenseed/internal/releases"
)

func Fingerprint(p models.ParsedRelease) string {
	p.DisplayGroup = ""
	p.Site = ""
	p.Checksum = ""
	if p.ContentID != "" {
		p.Title = ""
		p.Year = 0
	}
	if strings.HasPrefix(p.Structure, "episodes:") {
		p.AbsoluteEpisodes = nil
	}
	p.Title = releases.TitleKey(p.Title)
	b, _ := json.Marshal(p)
	return string(b)
}
func Complete(r models.Result) bool {
	p := r.Parsed
	return r.IdentityVerified && p.Title != "" && p.Resolution != "" && p.ReleaseType != "OTHER" && p.Source != "UNKNOWN" && p.Group != "" && p.VideoCodec != "" && p.Revision > 0 && !strings.Contains(p.Structure, "unknown")
}
func CompareWithRules(a, b models.Result, rules models.MatchingRules) (string, []string) {
	return CompareFingerprinted(a, b, Fingerprint(a.Parsed), Fingerprint(b.Parsed), rules)
}

// CompareFingerprinted is CompareWithRules for callers that already hold both
// results' fingerprints, so repeated comparisons skip re-encoding them.
func CompareFingerprinted(a, b models.Result, fingerprintA, fingerprintB string, rules models.MatchingRules) (string, []string) {
	if fingerprintA != fingerprintB {
		x, y := a.Parsed, b.Parsed
		if x.MediaType != y.MediaType || x.Structure != y.Structure || x.Resolution != y.Resolution || x.ReleaseType != y.ReleaseType || x.Source != y.Source || x.Correction != y.Correction || x.Revision != y.Revision || x.Hybrid != y.Hybrid || (x.ContentID != "" && y.ContentID != "" && x.ContentID != y.ContentID) || (x.Group != "" && y.Group != "" && x.Group != y.Group) || strings.Join(x.Edition, "|") != strings.Join(y.Edition, "|") || strings.Join(x.Cut, "|") != strings.Join(y.Cut, "|") {
			return "REJECTED", []string{"hard_metadata_mismatch"}
		}
		// Media type and structure agree past the check above. A shared title
		// with known, different years is a remake unless verified IDs agree.
		sharedID := x.ContentID != "" && x.ContentID == y.ContentID
		sameTitle := releases.TitleKey(x.Title) == releases.TitleKey(y.Title) && (sharedID || x.Year == 0 || y.Year == 0 || x.Year == y.Year)
		if !sameTitle && !sharedID {
			return "REJECTED", []string{"different_content_or_structure"}
		}
		if evidence := missingMetadata(x, y); len(evidence) > 0 {
			return "POSSIBLE", append(evidence, "verification_required")
		}
		if sameTitle {
			return "RELATED", []string{"different_release_metadata"}
		}
		return "RELATED", []string{"same_verified_content_different_metadata"}
	}
	e := []string{"same_title", "same_resolution", "same_source", "same_release_type", "same_group", "same_codec", "same_hdr", "same_edition", "same_audio", "same_tv_structure"}
	if a.Parsed.ContentID != "" && a.Parsed.ContentID == b.Parsed.ContentID && releases.TitleKey(a.Parsed.Title) != releases.TitleKey(b.Parsed.Title) {
		e[0] = "alternate_titles_same_verified_id"
	}
	e = append(e, "same_correction_revision", "same_hybrid_status")
	if !Complete(a) || !Complete(b) {
		return "POSSIBLE", append(e, "metadata_or_identity_unverified")
	}
	e = append(e, "same_content_identity")
	if a.Size <= 0 || b.Size <= 0 {
		return "POSSIBLE", append(e, "size_unknown")
	}
	if a.Size == b.Size {
		return "EXACT", append(e, "exact_positive_byte_size")
	}
	if rules.SizeTolerancePercent > 0 && math.Abs(float64(a.Size)-float64(b.Size))/float64(max(a.Size, b.Size))*100 <= rules.SizeTolerancePercent {
		return "STRONG", append(e, "size_within_configured_tolerance", "verification_required")
	}
	return "POSSIBLE", append(e, "reported_size_differs", "verification_required")
}

func missingMetadata(a, b models.ParsedRelease) []string {
	sameID := a.ContentID != "" && a.ContentID == b.ContentID
	if strings.Join(a.HDR, "|") != strings.Join(b.HDR, "|") || strings.Join(a.Language, "|") != strings.Join(b.Language, "|") || (!sameID && a.Year != b.Year && a.Year != 0 && b.Year != 0) {
		return nil
	}
	evidence := []string{}
	for _, field := range []struct{ name, a, b string }{{"codec", a.VideoCodec, b.VideoCodec}, {"bit_depth", a.BitDepth, b.BitDepth}, {"audio", a.AudioCodec, b.AudioCodec}, {"channels", a.AudioChannels, b.AudioChannels}, {"service", a.Service, b.Service}} {
		if field.a == field.b {
			continue
		}
		if field.a != "" && field.b != "" {
			return nil
		}
		evidence = append(evidence, "missing_"+field.name)
	}
	return evidence
}
func ConfidenceRank(s string) int {
	switch s {
	case "EXACT":
		return 5
	case "STRONG":
		return 4
	case "POSSIBLE":
		return 3
	case "RELATED":
		return 2
	default:
		return 1
	}
}
