package releases

import (
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/autobrr/rls"
	"golang.org/x/text/unicode/norm"
	"ravenseed/internal/models"
)

var separators = regexp.MustCompile(`[._ -]+`)
var titleSeparators = regexp.MustCompile(`[\s._:：–—-]+`)
var tvToken = regexp.MustCompile(`(?i)\bS(\d{1,3})(?:[ ._-]*E\d{1,4})?(?:[ ._-]*(?:E|S|-)\d{1,4})*\b`)
var episodeToken = regexp.MustCompile(`(?i)\bS\d+[ ._-]*E\d+|\b\d{1,2}x\d{2,3}\b`)

// COMPLETE marks a full disc only next to BLURAY; "S01.COMPLETE" is a pack.
var fullDisc = regexp.MustCompile(`(?i)\b(BD(?:25|50|66|100)|BDMV|COMPLETE[ ._-]*(?:UHD[ ._-]*)?BLU[ ._-]*RAY|BLU[ ._-]*RAY[ ._-]*COMPLETE|FULL[ ._-]*BLU[ ._-]*RAY|ISO)\b`)
var explicitDisc = regexp.MustCompile(`(?i)\b(BD(?:25|50|66|100)|BDMV|ISO)\b`)
var dvdTag = regexp.MustCompile(`(?i)\bDVD(?:[59]|R)?\b`)

// encodeCodec matches encoder, bit-depth and lossy audio tags that full discs
// do not carry.
var encodeCodec = regexp.MustCompile(`(?i)\b(?:x26[45]|H[ ._-]?26[45]|AV1|ENCODE|10[ ._-]?BIT|AAC|OPUS)\b`)
var discCodec = regexp.MustCompile(`(?i)\b(?:HEVC|AVC|VC[ ._-]?1|MPEG[ ._-]?2)\b`)
var codecHEVC = regexp.MustCompile(`(?i)\b(?:x265|h[ ._-]?265|hevc)\b`)
var codecAVC = regexp.MustCompile(`(?i)\b(?:x264|h[ ._-]?264|avc)\b`)
var bracketGroup = regexp.MustCompile(`^\[([A-Za-z][A-Za-z0-9 _-]{1,40})\]\s+`)
var trailingBracketGroup = regexp.MustCompile(`-\[([A-Za-z0-9][A-Za-z0-9_.-]{0,40})\]\s*$`)
var hyphenGroup = regexp.MustCompile(`-[A-Za-z0-9_.]+\s*$`)

// splitGroup and dottedGroup recover group names that contain the separators
// rls splits on, such as D-Z0N3 and E.N.D.
var splitGroup = regexp.MustCompile(`-([A-Za-z0-9]-[A-Za-z0-9]{2,20})\s*$`)
var dottedGroup = regexp.MustCompile(`-((?:[A-Za-z]\.){1,5}[A-Za-z])\s*$`)

// titleStructure finds rls title leftovers such as "S01E01v2" or "Seasons 1-3".
var titleStructure = regexp.MustCompile(`(?i)\s(?:S\d{1,3}(?:E\d{1,4})*(?:v\d+)?|Seasons?\s+\d{1,3})\b`)
var wordToken = regexp.MustCompile(`[\p{L}\p{N}]+`)

// maxParsedTitle bounds the work done on hostile indexer titles.
const maxParsedTitle = 512

func Parse(result *models.Result, media models.Media) {
	ParseWithRules(result, media, DefaultRules())
}

func ParseWithRules(result *models.Result, media models.Media, rules models.MatchingRules) {
	input := truncate(result.Title, maxParsedTitle)
	original := rls.ParseString(input)
	result.ParserOutput, _ = json.Marshal(original)
	title, site, evidence := cleanName(input, rules)
	result.NormalizationEvidence = evidence
	r := original
	if title != input {
		r = rls.ParseString(title)
	}
	u := strings.ToUpper(separators.ReplaceAllString(title, " "))
	compact := strings.ReplaceAll(u, " ", "")
	p := models.ParsedRelease{MediaType: media.Type, Title: cutTitle(r.Title), Year: r.Year, Season: r.Series, Episode: r.Episode, Structure: "movie", Resolution: strings.ToLower(r.Resolution), ReleaseType: "OTHER", Source: "UNKNOWN", VideoCodec: strings.ToUpper(strings.Join(r.Codec, "+")), BitDepth: r.BitDepth, HDR: normalized(r.HDR), AudioCodec: strings.ToUpper(strings.Join(r.Audio, "+")), AudioChannels: r.Channels, Edition: normalized(r.Edition), Cut: normalized(r.Cut), Site: r.Site, Service: strings.ToUpper(r.Collection), Language: normalized(r.Language)}
	p.DisplayGroup = releaseGroup(input, title, r.Group)
	p.Group = strings.ToUpper(p.DisplayGroup)
	p.Checksum = strings.ToUpper(original.Sum)
	if p.Resolution == "4k" {
		p.Resolution = "2160p"
	}
	if p.Resolution == "8k" {
		p.Resolution = "4320p"
	}
	if codecHEVC.MatchString(title) {
		p.VideoCodec = "HEVC"
	} else if codecAVC.MatchString(title) {
		p.VideoCodec = "AVC"
	}
	switch {
	case strings.Contains(compact, "WEBRIP"):
		p.Source = "WEB"
		p.ReleaseType = "WEBRIP"
	case strings.Contains(compact, "WEBDL") || strings.EqualFold(r.Source, "WEB"):
		p.Source = "WEB_DL"
		p.ReleaseType = "WEB_DL"
	case strings.Contains(compact, "BLURAY") || strings.Contains(compact, "BDREMUX") || (explicitDisc.MatchString(u) && !dvdTag.MatchString(u)):
		p.Source = "BLURAY"
		if strings.Contains(compact, "UHD") || strings.Contains(compact, "ULTRAHD") {
			p.Source = "UHD_BLURAY"
		}
		p.ReleaseType = "ENCODE"
		if strings.Contains(compact, "REMUX") {
			p.ReleaseType = "REMUX"
		} else if (fullDisc.MatchString(u) || discCodec.MatchString(u)) && !encodeCodec.MatchString(u) {
			p.ReleaseType = "FULL_DISC"
		}
	case strings.Contains(compact, "HDTV"):
		p.Source = "HDTV"
		p.ReleaseType = "HDTV"
	case r.Source != "":
		p.Source = strings.ToUpper(r.Source)
	}
	hdr := []string{}
	for _, v := range p.HDR {
		switch strings.ReplaceAll(v, " ", "") {
		case "DV", "DOVI", "DOLBYVISION":
			hdr = append(hdr, "DV")
		case "HDR10+", "HDR10PLUS":
			hdr = append(hdr, "HDR10+")
		case "HDR", "HDR10":
			hdr = append(hdr, "HDR10")
		default:
			hdr = append(hdr, v)
		}
	}
	p.HDR = normalized(hdr)
	result.Parsed = p
	applyRules(result, media, rules, title, site)
}

func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	for limit > 0 && !utf8.RuneStart(s[limit]) {
		limit--
	}
	return s[:limit]
}

// cutTitle drops season and episode tokens rls leaves in titles, so
// "Show S01E01v2" and "Show Seasons 1-3" key like "Show".
func cutTitle(title string) string {
	if loc := titleStructure.FindStringIndex(title); loc != nil {
		return strings.TrimSpace(title[:loc[0]])
	}
	return title
}

// releaseGroup corrects rls group extraction for upload suffixes, groups
// containing separators, trailing bracketed groups and anime names whose
// group is the leading bracket.
func releaseGroup(original, clean, group string) string {
	if m := trailingBracketGroup.FindStringSubmatch(clean); len(m) > 1 {
		return m[1]
	}
	if m := dottedGroup.FindStringSubmatch(clean); len(m) > 1 {
		return m[1]
	}
	// "DTS-X" before a group is audio, not part of the group name.
	if loc := splitGroup.FindStringSubmatchIndex(clean); loc != nil && group != "" && !strings.HasSuffix(strings.ToUpper(clean[:loc[0]]), "DTS") {
		if candidate := clean[loc[2]:loc[3]]; strings.HasSuffix(strings.ToUpper(candidate), "-"+strings.ToUpper(group)) {
			return candidate
		}
	}
	if m := bracketGroup.FindStringSubmatch(original); len(m) > 1 && (group == "" || !hyphenGroup.MatchString(clean)) {
		return m[1]
	}
	return group
}

// titleEnd returns the byte offset in text just after the parsed title, so
// tags that are title words ("Hybrid.2013", "Proper.Title.2013") are skipped.
func titleEnd(text, title string) int {
	want := wordToken.FindAllString(title, -1)
	if len(want) == 0 {
		return 0
	}
	locs := wordToken.FindAllStringIndex(text, -1)
	for i := 0; i+len(want) <= len(locs); i++ {
		match := true
		for j, w := range want {
			if !strings.EqualFold(text[locs[i+j][0]:locs[i+j][1]], w) {
				match = false
				break
			}
		}
		if match {
			return locs[i+len(want)-1][1]
		}
	}
	return 0
}

func normalized(v []string) []string {
	out := make([]string, 0, len(v))
	for _, s := range v {
		if s = strings.ToUpper(strings.TrimSpace(s)); s != "" {
			out = append(out, s)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// TitleKey folds punctuation, apostrophes, accents and "&" so tracker title
// spellings compare equal. It never removes words.
func TitleKey(s string) string {
	s = foldAccents(titleQuotes.Replace(s))
	return strings.ToLower(strings.TrimSpace(titleSeparators.ReplaceAllString(s, " ")))
}

var titleQuotes = strings.NewReplacer("'", "", "’", "", "‘", "", "`", "", "&", " and ")

func foldAccents(s string) string {
	ascii := true
	for i := 0; i < len(s) && ascii; i++ {
		ascii = s[i] < 0x80
	}
	if ascii {
		return s
	}
	var b strings.Builder
	for _, r := range norm.NFD.String(s) {
		if !unicode.Is(unicode.Mn, r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Episodic reports TV episode markers, including air dates, before a media
// type is known. Plain years are not episode markers.
func Episodic(title string) bool {
	return tvToken.MatchString(title) || episodeToken.MatchString(title) || dateEpisode.MatchString(title)
}
