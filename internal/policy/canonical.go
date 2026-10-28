package policy

import (
	"ravenseed/internal/models"
	"regexp"
	"slices"
	"strings"
)

// Policy values are typed by hand, so they are read in the parser's
// canonical forms: "HDR10+DV" is "DV+HDR10", "x264" is AVC and "Blu-ray" is
// BLURAY.

var hdrSeparators = regexp.MustCompile(`[\s,/]+`)

var hdrAliases = map[string]string{"DOVI": "DV", "DOLBYVISION": "DV", "HDR": "HDR10", "HDR10PLUS": "HDR10+"}

// canonicalHDR returns an HDR combination as the parser joins one: aliases
// resolved, sorted and joined with "+". "+" both joins formats and ends
// HDR10+, so an empty piece after HDR10 ("DV+HDR10+", "HDR10++DV") marks
// HDR10+.
func canonicalHDR(s string) string {
	parts := []string{}
	for _, piece := range strings.Split(hdrSeparators.ReplaceAllString(strings.ToUpper(s), "+"), "+") {
		if piece == "" {
			if n := len(parts); n > 0 && parts[n-1] == "HDR10" {
				parts[n-1] = "HDR10+"
			}
			continue
		}
		if alias, ok := hdrAliases[piece]; ok {
			piece = alias
		}
		parts = append(parts, piece)
	}
	slices.Sort(parts)
	return strings.Join(slices.Compact(parts), "+")
}

var codecAliases = map[string]string{"X264": "AVC", "H264": "AVC", "H.264": "AVC", "X265": "HEVC", "H265": "HEVC", "H.265": "HEVC"}

func canonicalCodec(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	if alias, ok := codecAliases[s]; ok {
		return alias
	}
	return s
}

var sourceSeparators = regexp.MustCompile(`[\s._-]+`)

var sourceAliases = map[string]string{"BLURAY": "BLURAY", "BD": "BLURAY", "UHDBLURAY": "UHD_BLURAY", "WEBDL": "WEB_DL", "WEBRIP": "WEB", "WEB": "WEB", "HDTV": "HDTV"}

func canonicalSource(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	if alias, ok := sourceAliases[sourceSeparators.ReplaceAllString(s, "")]; ok {
		return alias
	}
	return s
}

// sourceRank is the source's position in the policy's priority, or -1.
func sourceRank(v models.IndexerPolicy, source string) int {
	source = canonicalSource(source)
	return slices.IndexFunc(v.SourcePriority, func(s string) bool { return canonicalSource(s) == source })
}

// evaluableTV reports seasonless TV structures the evaluator can compare:
// S00 specials and air dates. Absolute numbering stays unevaluated: a target
// may name the same episode by season, which would read as a missing slot.
func evaluableTV(p models.ParsedRelease) bool {
	switch {
	case strings.HasPrefix(p.Structure, "episodes:"), strings.HasPrefix(p.Structure, "seasons:"):
		return len(p.Seasons) > 0
	case strings.HasPrefix(p.Structure, "date:"):
		return true
	}
	return false
}
