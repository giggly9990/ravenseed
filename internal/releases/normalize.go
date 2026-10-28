package releases

import (
	"regexp"
	"strconv"
	"strings"
	"sync"

	"ravenseed/internal/models"
)

var trailingSite = regexp.MustCompile(`(?i)\s*[\[{]((?:[a-z0-9-]+\.)+[a-z]{2,})[\]}](?:\.(?:torrent|mkv|mp4))?\s*$`)
var leadingSite = regexp.MustCompile(`(?i)^\s*\[((?:[a-z0-9-]+\.)+[a-z]{2,})\]\s*`)
var groupAtSite = regexp.MustCompile(`(?i)-([a-z0-9_-]+)@((?:[a-z0-9-]+\.)+[a-z]{2,})(?:\.torrent)?$`)
var checksum = regexp.MustCompile(`(?i)\s*\[[0-9a-f]{8}\](?:\.(?:mkv|mp4|torrent))?\s*$`)

// correctionTag reads "REPACK2" and "REPACK.2" as revision 2. A separated
// number needs one or two digits and no ".N" after it, so "REPACK.5.1" is
// REPACK before audio channels and "Repack.2013" is a year.
var correctionTag = regexp.MustCompile(`(?i)\b(REPACK|RERIP|PROPER|RE[ ._-]?ISSUE)(?:([0-9]+)|[ ._-]([0-9]{1,2})(?:[ ._-]([0-9])\b)?)?\b`)
var hybridTag = regexp.MustCompile(`(?i)\bHYBRID\b`)
var webDLTag = regexp.MustCompile(`(?i)\bWEB[ ._-]*DL\b`)
var webRipTag = regexp.MustCompile(`(?i)\bWEB[ ._-]*RIP\b`)
var codecAV1 = regexp.MustCompile(`(?i)\bAV1\b`)
var pq10Tag = regexp.MustCompile(`(?i)\bPQ10\b`)

// uploadSuffix matches indexer and usenet suffixes appended after the group.
var uploadSuffix = regexp.MustCompile(`(?i)-(?:sample|AsRequested|AlternativeToRequested|Obfuscated|Scrambled|postbot|xpost|Rakuv[a-z0-9]*|WhiteRev|BUYMORE)(?:\.(?:mkv|mp4|torrent))?\s*$`)

var tagSeparators = regexp.MustCompile(`[ ._-]+`)
var correctionSpaces = strings.NewReplacer(".", " ", "_", " ")
var correctionKind = strings.NewReplacer(" ", "", "-", "", "_", "", ".", "")

var serviceAliases = map[string]string{"NETFLIX": "NF", "NF": "NF", "AMAZON": "AMZN", "AMAZONPRIME": "AMZN", "AMAZON PRIME": "AMZN", "AMZN": "AMZN", "DISNEYPLUS": "DSNP", "DISNEY+": "DSNP", "DISNEY PLUS": "DSNP", "DSNP": "DSNP", "HBO MAX": "HMAX", "HBOMAX": "HMAX", "HMAX": "HMAX", "MAX": "HMAX", "APPLETV+": "ATVP", "APPLE TV+": "ATVP", "APPLETV": "ATVP", "ATVP": "ATVP", "ATV": "ATVP", "PARAMOUNTPLUS": "PMTP", "PARAMOUNT+": "PMTP", "PMTP": "PMTP", "PEACOCK": "PCOK", "PCOK": "PCOK", "CRUNCHYROLL": "CR", "CR": "CR", "HULU": "HULU"}

// serviceTail lists names searched after the resolution when rls finds no
// service. Bare "CR" and "ATV" are left out as too ambiguous.
var serviceTail = []string{"NETFLIX", "AMAZON PRIME", "AMAZON", "DISNEY PLUS", "DISNEY+", "HBO MAX", "HBOMAX", "APPLE TV+", "APPLETV+", "PARAMOUNT+", "PEACOCK", "CRUNCHYROLL", "NF", "AMZN", "DSNP", "HMAX", "MAX", "ATVP", "PMTP", "PCOK", "HULU"}

type ignoredTagPatterns struct{ bracket, token *regexp.Regexp }

// ignoredTagCache holds compiled patterns per tag; tags are validated and
// capped by NormalizeRules, so the cache stays small.
var ignoredTagCache sync.Map

func ignoredTagRegexps(tag string) ignoredTagPatterns {
	if cached, ok := ignoredTagCache.Load(tag); ok {
		return cached.(ignoredTagPatterns)
	}
	words := tagSeparators.Split(tag, -1)
	for i, word := range words {
		words[i] = regexp.QuoteMeta(word)
	}
	literal := strings.Join(words, `[ ._-]+`)
	patterns := ignoredTagPatterns{
		bracket: regexp.MustCompile(`(?i)(?:\[` + literal + `\]|\{` + literal + `\})`),
		token:   regexp.MustCompile(`(?i)(^|[ ._])` + literal + `([ ._]|$)`),
	}
	ignoredTagCache.Store(tag, patterns)
	return patterns
}

func cleanName(title string, rules models.MatchingRules) (string, string, []string) {
	clean, site := title, ""
	evidence := []string{}
	if m := leadingSite.FindStringSubmatch(clean); len(m) > 1 {
		site = m[1]
		clean = leadingSite.ReplaceAllString(clean, "")
		evidence = append(evidence, "site_prefix_excluded")
	}
	if m := trailingSite.FindStringSubmatch(clean); len(m) > 1 {
		site = m[1]
		clean = strings.TrimSpace(trailingSite.ReplaceAllString(clean, ""))
		evidence = append(evidence, "site_suffix_excluded")
	}
	if m := groupAtSite.FindStringSubmatch(clean); len(m) > 2 {
		site = m[2]
		clean = groupAtSite.ReplaceAllString(clean, "-$1")
		evidence = append(evidence, "site_suffix_excluded")
	}
	if checksum.MatchString(clean) {
		clean = checksum.ReplaceAllString(clean, "")
		evidence = append(evidence, "anime_checksum_excluded")
	}
	clean = uploadSuffix.ReplaceAllString(clean, "")
	for _, tag := range rules.IgnoredTags {
		patterns := ignoredTagRegexps(tag)
		// A leading bracket can identify the release group.
		clean = patterns.bracket.ReplaceAllStringFunc(clean, func(match string) string {
			if strings.HasPrefix(title, match) {
				return match
			}
			evidence = append(evidence, "ignored_cosmetic_tag:"+tag)
			return " "
		})
		if patterns.token.MatchString(clean) {
			clean = patterns.token.ReplaceAllString(clean, "$1$2")
			evidence = append(evidence, "ignored_cosmetic_tag:"+tag)
		}
	}
	clean = webDLTag.ReplaceAllString(clean, "WEB-DL")
	clean = webRipTag.ReplaceAllString(clean, "WEBRip")
	return clean, site, evidence
}

func normalizeService(service, title string) string {
	if canonical, ok := serviceAliases[strings.ToUpper(service)]; ok {
		return canonical
	}
	if service != "" {
		return strings.ToUpper(service)
	}
	if loc := resolutionTag.FindStringIndex(title); loc != nil {
		rest := title[loc[1]:]
		// The trailing group is not a service: "-MAX" names a group.
		if dash := strings.LastIndex(rest, "-"); dash > 0 {
			rest = rest[:dash]
		}
		tail := strings.ToUpper(separators.ReplaceAllString(rest, " "))
		for _, alias := range serviceTail {
			if strings.Contains(" "+tail+" ", " "+alias+" ") {
				return serviceAliases[alias]
			}
		}
	}
	return ""
}

func applyRules(result *models.Result, media models.Media, rules models.MatchingRules, title, site string) {
	p := &result.Parsed
	if token := checksum.FindString(result.Title); token != "" && p.Checksum == "" {
		start := strings.Index(token, "[")
		if start >= 0 && len(token) >= start+9 {
			p.Checksum = strings.ToUpper(token[start+1 : start+9])
		}
	}
	if result.IdentityVerified && rules.MatchAlternateTitles {
		p.ContentID = MediaKey(media)
		result.NormalizationEvidence = append(result.NormalizationEvidence, "verified_media_identity")
	}
	if alias, ok := rules.GroupAliases[p.Group]; ok {
		p.Group = alias
		result.NormalizationEvidence = append(result.NormalizationEvidence, "explicit_group_alias")
	}
	if site != "" {
		p.Site = site
	}
	service := normalizeService(p.Service, title)
	if service != p.Service {
		result.NormalizationEvidence = append(result.NormalizationEvidence, "web_service_alias")
	}
	p.Service = service
	p.Repack = false
	p.Proper = false
	p.Revision = 1
	p.Correction = ""
	// Tags inside the title ("Hybrid.2013", "Proper.Title.2013") are words of
	// the name, not release flags.
	tags := correctionSpaces.Replace(title)
	tags = tags[titleEnd(tags, p.Title):]
	p.Hybrid = hybridTag.MatchString(tags)
	corrections := []string{}
	for _, m := range correctionTag.FindAllStringSubmatch(tags, -1) {
		kind := correctionKind.Replace(strings.ToUpper(m[1]))
		corrections = append(corrections, kind)
		if kind == "PROPER" {
			p.Proper = true
		} else if kind == "REPACK" || kind == "RERIP" {
			p.Repack = true
		}
		if m[2] != "" {
			p.Revision = revisionNumber(p.Revision, m[2])
		} else if m[3] != "" && m[4] == "" {
			p.Revision = revisionNumber(p.Revision, m[3])
		}
	}
	p.Correction = strings.Join(normalized(corrections), "+")
	if p.Correction != "" {
		result.NormalizationEvidence = append(result.NormalizationEvidence, "correction_revision_preserved")
	}
	if p.Hybrid {
		result.NormalizationEvidence = append(result.NormalizationEvidence, "hybrid_metadata_preserved")
	}
	structure(p, title, media, rules, &result.NormalizationEvidence)
	if codecAV1.MatchString(title) {
		p.VideoCodec = "AV1"
	}
	if pq10Tag.MatchString(title) {
		p.HDR = normalized(append(p.HDR, "PQ10"))
	}
}

func revisionNumber(current int, text string) int {
	n, err := strconv.Atoi(text)
	if current < 0 || err != nil || n < 1 || n > 99 {
		return -1
	}
	return max(current, n)
}
