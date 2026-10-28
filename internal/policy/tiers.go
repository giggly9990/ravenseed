package policy

import (
	"fmt"
	"ravenseed/internal/models"
	"ravenseed/internal/releases"
	"regexp"
	"slices"
	"strings"
)

// Context carries title facts a release name lacks. RuntimeMinutes is the
// movie or average episode runtime; SeasonEpisodes counts episodes per season.
type Context struct {
	RuntimeMinutes int
	SeasonEpisodes map[int]int
	// SourceVideoMbps is the candidate's measured video bitrate (MediaInfo),
	// used instead of the estimate; 0 when unknown.
	SourceVideoMbps float64
}

// tierMargin keeps estimates near a tier limit undecided: audio, multiple
// tracks and runtime rounding make the video bitrate approximate.
const tierMargin = 0.10

// audioMbps estimates the main audio bitrate from the release name.
func audioMbps(p models.ParsedRelease) float64 {
	a, multi := p.AudioCodec, p.AudioChannels != "" && p.AudioChannels != "1.0" && p.AudioChannels != "2.0"
	pick := func(stereo, surround float64) float64 {
		if multi {
			return surround
		}
		return stereo
	}
	switch {
	case strings.Contains(a, "TRUEHD"):
		return pick(2.0, 4.5)
	case strings.Contains(a, "DTS-HD.MA"), strings.Contains(a, "DTS-X"):
		return pick(1.8, 4.0)
	case strings.Contains(a, "PCM"):
		return pick(2.3, 6.9)
	case strings.Contains(a, "FLAC"):
		return pick(0.9, 3.0)
	case strings.Contains(a, "DTS"):
		return 1.5
	case strings.Contains(a, "DDP"):
		return pick(0.256, 1.0)
	case strings.Contains(a, "DD"):
		return pick(0.224, 0.64)
	case strings.Contains(a, "AAC"), strings.Contains(a, "OPUS"):
		return pick(0.2, 0.4)
	}
	return 0.8
}

// playMinutes is the release running time, or 0 when it is unknown.
func playMinutes(p models.ParsedRelease, ctx Context) float64 {
	if ctx.RuntimeMinutes <= 0 {
		return 0
	}
	switch {
	case p.MediaType == "movie":
		return float64(ctx.RuntimeMinutes)
	case strings.HasPrefix(p.Structure, "episodes:"):
		return float64(ctx.RuntimeMinutes * len(p.Episodes))
	case strings.HasPrefix(p.Structure, "seasons:"):
		episodes := 0
		for _, season := range p.Seasons {
			if ctx.SeasonEpisodes[season] <= 0 {
				return 0
			}
			episodes += ctx.SeasonEpisodes[season]
		}
		return float64(ctx.RuntimeMinutes * episodes)
	}
	return 0
}

// bitrateTier places an encode in the compact or transparent tier. It returns
// "" when no tier applies and "unknown" with a reason when it cannot tell.
// measured is a MediaInfo video bitrate in Mbps, or 0 to estimate one.
func bitrateTier(v models.IndexerPolicy, r models.Result, ctx Context, measured float64) (string, string) {
	p := r.Parsed
	if p.ReleaseType != "ENCODE" {
		return "", ""
	}
	pos := slices.IndexFunc(v.EncodeBitrateTiers, func(t models.EncodeBitrateTier) bool { return strings.EqualFold(t.Resolution, p.Resolution) })
	if pos < 0 {
		return "", ""
	}
	limit := v.EncodeBitrateTiers[pos].MaxVideoMbps
	if measured > 0 {
		// A measured bitrate needs no margin: "up to the limit" is compact.
		if measured <= limit {
			return "compact", ""
		}
		return "transparent", ""
	}
	minutes := playMinutes(p, ctx)
	if r.Size <= 0 || minutes <= 0 {
		return "unknown", "Runtime or size is unavailable, so the encode size tier is unknown"
	}
	video := float64(r.Size)*8/(minutes*60)/1e6 - audioMbps(p)
	switch {
	case video <= limit*(1-tierMargin):
		return "compact", ""
	case video >= limit*(1+tierMargin):
		return "transparent", ""
	}
	return "unknown", fmt.Sprintf("Estimated video bitrate %.1f Mbps is too close to the %.0f Mbps tier limit", video, limit)
}

// inSizeScope reports whether the encode size boundary applies to a release.
func inSizeScope(v models.IndexerPolicy, p models.ParsedRelease) bool {
	if len(v.EncodeSizeScope) == 0 {
		return true
	}
	return slices.ContainsFunc(v.EncodeSizeScope, func(s string) bool {
		resolution, codec, _ := strings.Cut(s, ":")
		return strings.EqualFold(resolution, p.Resolution) && canonicalCodec(codec) == canonicalCodec(p.VideoCodec)
	})
}

// likelySDR covers names where an absent HDR tag means SDR in practice: AVC
// at 1080p or below.
func likelySDR(p models.ParsedRelease) bool {
	i := slices.Index(releases.Resolutions, p.Resolution)
	return p.VideoCodec == "AVC" && i >= 0 && i <= slices.Index(releases.Resolutions, "1080p")
}

// hdrUncertain reports whether an absent HDR tag leaves two releases'
// HDR formats open.
func hdrUncertain(a, b models.ParsedRelease) bool {
	if len(a.HDR) > 0 && len(b.HDR) > 0 {
		return false
	}
	return !(len(a.HDR) == 0 && len(b.HDR) == 0 && likelySDR(a) && likelySDR(b))
}

func dualAudio(p models.ParsedRelease) bool {
	return strings.Contains(p.AudioCodec, "DUAL.AUDIO") || slices.Contains(p.Language, "DL") || slices.Contains(p.Language, "MULTI")
}

// LosslessAudio reports a lossless main audio codec (TrueHD, DTS-HD MA,
// DTS:X, FLAC or PCM).
func LosslessAudio(p models.ParsedRelease) bool {
	for _, codec := range []string{"TRUEHD", "DTS-HD.MA", "DTS-X", "FLAC", "PCM"} {
		if strings.Contains(p.AudioCodec, codec) {
			return true
		}
	}
	return false
}

// internal reports a release by one of this target tracker's own groups.
func internal(v models.IndexerPolicy, p models.ParsedRelease) bool {
	return p.Group != "" && slices.ContainsFunc(v.InternalGroups, func(g string) bool { return strings.EqualFold(g, p.Group) })
}

// Banned reports a release from a group this target tracker refuses. Group
// names can contain separators (YTS.MX, BEN THE MEN, ALCAIDE_KIRA) or a tag
// (WEASLEY[HONE]) the parser splits off, so the release name's ending counts
// too: "-" followed by the listed name, ignoring case and separators.
func Banned(v models.IndexerPolicy, r models.Result) bool {
	if len(v.BannedGroups) == 0 {
		return false
	}
	name := bannedKey(fileExtension.ReplaceAllString(r.Title, ""))
	return slices.ContainsFunc(v.BannedGroups, func(g string) bool {
		if strings.TrimSpace(g) == "" {
			return false
		}
		return (r.Parsed.Group != "" && strings.EqualFold(g, r.Parsed.Group)) || strings.HasSuffix(name, "-"+bannedKey(g))
	})
}

var bannedSeparators = regexp.MustCompile(`[\s._]+`)
var fileExtension = regexp.MustCompile(`(?i)(?:\.(?:mkv|mp4|torrent))+$`)

func bannedKey(s string) string {
	return strings.ToUpper(bannedSeparators.ReplaceAllString(strings.TrimSpace(s), " "))
}

// upgradeReview applies the trump review checks to an upgrade read from the
// target's side: an HDR ranking needs comparable video quality when the
// policy asks for it, and an unstated HDR format leaves the ranking open.
func upgradeReview(v models.IndexerPolicy, rules models.SlotAxes, candidate, target models.ParsedRelease, reasons []models.PolicyImprovement) string {
	if len(reasons) == 0 {
		return ""
	}
	if v.ReviewHDRComparability && slices.ContainsFunc(reasons, func(r models.PolicyImprovement) bool { return r.Type == "hdr" }) {
		return "An HDR format ranks higher, but comparable video quality is not established; compare before replacing"
	}
	if rules.HDR && hdrUncertain(candidate, target) {
		return "One release does not state its HDR format; confirm both before replacing"
	}
	return ""
}
