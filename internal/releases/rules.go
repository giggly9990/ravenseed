package releases

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"ravenseed/internal/models"
)

const EngineVersion = 7

var groupName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)
var cosmeticTag = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_ -]{0,39}$`)
var imdbIdentifier = regexp.MustCompile(`^tt[0-9]{7,10}$`)
var structuralTag = regexp.MustCompile(`(?i)^(?:s\d|e\d|v\d|\d{3,4}[pi]|\d{1,2}bit)`)
var protectedTags = strings.Fields("REMUX ENCODE BLURAY BLU RAY UHD ULTRAHD WEB WEBDL WEB DL WEBRIP HDTV BDMV BD25 BD50 BD66 BD100 DVD DVDRIP ISO COMPLETE FULL DISC REPACK RERIP PROPER REISSUE RE-ISSUE HYBRID REMASTERED EXTENDED THEATRICAL DIRECTORS CUT UNRATED UNCUT SPECIAL EDITION IMAX CRITERION DV DOVI DOLBY VISION HDR HDR10 HDR10PLUS PQ10 SDR HLG HEVC AVC X264 X265 H264 H265 AV1 VC1 MPEG2 DTS DTSHD DDP DD AC3 EAC3 AAC FLAC TRUEHD ATMOS MULTI DUAL DUBBED SUBBED NF NETFLIX AMZN AMAZON DSNP DISNEY HMAX HBO MAX ATVP APPLE HULU PCOK PMTP CR CRUNCHYROLL")

func DefaultRules() models.MatchingRules {
	return models.MatchingRules{MatchAlternateTitles: true, SizeTolerancePercent: 0.1, GroupAliases: map[string]string{}, IgnoredTags: []string{}, EpisodeMappings: []models.EpisodeMapping{}}
}

func NormalizeRules(input *models.MatchingRules) (models.MatchingRules, error) {
	if input == nil {
		return DefaultRules(), nil
	}
	r := *input
	if math.IsNaN(r.SizeTolerancePercent) || math.IsInf(r.SizeTolerancePercent, 0) || r.SizeTolerancePercent < 0 || r.SizeTolerancePercent > 1 {
		return r, fmt.Errorf("sizeTolerancePercent must be between 0 and 1")
	}
	if len(r.GroupAliases) > 100 || len(r.IgnoredTags) > 100 || len(r.EpisodeMappings) > 1000 {
		return r, fmt.Errorf("matching rules exceed the supported limits")
	}
	r.GroupAliases = map[string]string{}
	for from, to := range input.GroupAliases {
		from, to = strings.ToUpper(strings.TrimSpace(from)), strings.ToUpper(strings.TrimSpace(to))
		if !groupName.MatchString(from) || !groupName.MatchString(to) {
			return r, fmt.Errorf("group aliases must contain letters, digits, underscores or hyphens")
		}
		if previous, exists := r.GroupAliases[from]; exists && previous != to {
			return r, fmt.Errorf("conflicting case-insensitive group alias: %s", from)
		}
		r.GroupAliases[from] = to
	}
	flat := map[string]string{}
	for from := range r.GroupAliases {
		seen := map[string]bool{from: true}
		to := r.GroupAliases[from]
		for {
			if seen[to] {
				return r, fmt.Errorf("group aliases contain a cycle at %s", from)
			}
			seen[to] = true
			next, ok := r.GroupAliases[to]
			if !ok {
				break
			}
			to = next
		}
		flat[from] = to
	}
	r.GroupAliases = flat
	r.IgnoredTags = []string{}
	for _, tag := range input.IgnoredTags {
		tag = strings.ToUpper(strings.TrimSpace(tag))
		if !cosmeticTag.MatchString(tag) || structuralTag.MatchString(tag) {
			return r, fmt.Errorf("invalid cosmetic tag: %s", tag)
		}
		for _, word := range separators.Split(tag, -1) {
			if slices.Contains(protectedTags, word) {
				return r, fmt.Errorf("cannot ignore matching metadata tag: %s", tag)
			}
		}
		r.IgnoredTags = append(r.IgnoredTags, tag)
	}
	slices.Sort(r.IgnoredTags)
	r.IgnoredTags = slices.Compact(r.IgnoredTags)
	r.EpisodeMappings = slices.Clone(input.EpisodeMappings)
	if r.EpisodeMappings == nil {
		r.EpisodeMappings = []models.EpisodeMapping{}
	}
	seen := map[string]bool{}
	targets := map[string]bool{}
	for _, m := range r.EpisodeMappings {
		if (m.IMDbID == "") == (m.TMDbID == 0) || m.TMDbID < 0 || (m.IMDbID != "" && !imdbIdentifier.MatchString(m.IMDbID)) || m.Absolute < 1 || m.Absolute > 9999 || m.Season < 0 || m.Season > 999 || m.Episode < 1 || m.Episode > 9999 {
			return r, fmt.Errorf("episode mappings require one media ID and valid absolute, season and episode numbers")
		}
		identity := MediaKey(models.Media{Type: "tv", IMDbID: m.IMDbID, TMDbID: m.TMDbID})
		key := fmt.Sprintf("%s/%d", identity, m.Absolute)
		target := fmt.Sprintf("%s/%d/%d", identity, m.Season, m.Episode)
		if seen[key] || targets[target] {
			return r, fmt.Errorf("duplicate or ambiguous episode mapping")
		}
		seen[key] = true
		targets[target] = true
	}
	slices.SortFunc(r.EpisodeMappings, func(a, b models.EpisodeMapping) int {
		ka := fmt.Sprintf("%s/%010d/%05d", a.IMDbID, a.TMDbID, a.Absolute)
		kb := fmt.Sprintf("%s/%010d/%05d", b.IMDbID, b.TMDbID, b.Absolute)
		return strings.Compare(ka, kb)
	})
	return r, nil
}

// NormalizeIMDbID returns an IMDb title ID as "tt" and at least seven
// digits, accepting a missing prefix or dropped leading zeros, or "" when s is
// not a positive IMDb number.
func NormalizeIMDbID(s string) string {
	digits := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(s)), "tt")
	if digits == "" || len(digits) > 10 || strings.Trim(digits, "0123456789") != "" {
		return ""
	}
	n, err := strconv.ParseInt(digits, 10, 64)
	if err != nil || n <= 0 {
		return ""
	}
	return fmt.Sprintf("tt%07d", n)
}

func MediaKey(m models.Media) string {
	if m.IMDbID != "" {
		return m.Type + ":imdb:" + m.IMDbID
	}
	if m.TMDbID > 0 {
		return m.Type + ":tmdb:" + strconv.Itoa(m.TMDbID)
	}
	return ""
}
