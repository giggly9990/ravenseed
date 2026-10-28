package releases

import (
	"regexp"
	"slices"
	"strings"
)

// ReleaseClasses lists the release classes the parser reports, best first.
var ReleaseClasses = []string{"FULL_DISC", "REMUX", "ENCODE", "WEB_DL", "WEBRIP", "HDTV", "OTHER"}

// Resolutions lists the resolutions the parser reports, lowest first.
var Resolutions = []string{"480p", "576p", "720p", "1080i", "1080p", "1440p", "2160p", "4320p"}

// ClassRank orders release classes: the best class ranks highest, OTHER
// ranks 1 and an unknown class 0.
func ClassRank(class string) int {
	if i := slices.Index(ReleaseClasses, class); i >= 0 {
		return len(ReleaseClasses) - i
	}
	return 0
}

// resolutionTag finds a resolution tag in a name: any listed height,
// progressive or interlaced.
var resolutionTag = regexp.MustCompile(`(?i)\b(?:` + strings.Join(resolutionHeights(), "|") + `)[pi]\b`)

// resolutionHeights lists each height in Resolutions once, highest first.
func resolutionHeights() []string {
	var heights []string
	for _, r := range slices.Backward(Resolutions) {
		if h := strings.TrimRight(r, "pi"); !slices.Contains(heights, h) {
			heights = append(heights, h)
		}
	}
	return heights
}
