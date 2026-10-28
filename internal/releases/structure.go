package releases

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"ravenseed/internal/models"
)

var seasonEpisode = regexp.MustCompile(`(?i)\bS(\d{1,3})\s*E(\d{1,4}(?:(?:\s*E|\s*-\s*(?:E)?)\d{1,4})*)(?:v(\d+))?\b`)

// xEpisode keeps seasons to two digits and episodes to three so frame sizes
// such as 640x360 are not read as episodes.
var xEpisode = regexp.MustCompile(`(?i)\b(\d{1,2})x(\d{1,3}(?:\s*-\s*\d{1,3})*)(?:v(\d+))?\b`)
var namedEpisode = regexp.MustCompile(`(?i)\bSeason\s+(\d{1,3})\s+Episode\s+(\d{1,4}(?:\s*-\s*\d{1,4})*)\b`)

// animeSeasonEpisode reads "Show S2 - 05" as season 2 episode 5.
var animeSeasonEpisode = regexp.MustCompile(`(?i)\bS(\d{1,2})\s+-\s+(\d{1,4})(?:v(\d+))?\b`)

// seasonPack needs an S on a spaced range end: "S1 - S3" is a range, while
// "S2 - 05" is an episode.
var seasonPack = regexp.MustCompile(`(?i)\bS(\d{1,3})(?:(?:-S?|\s*-\s*S)(\d{1,3}))?\b|\bSeasons?\s+(\d{1,3})(?:\s*-\s*(?:S|Seasons?\s+)?(\d{1,3}))?\b`)

// repeatedSeason matches "S01E01-S01E03" so the range keeps its middle.
var repeatedSeason = regexp.MustCompile(`(?i)\bS(\d{1,3})(\s*E\d{1,4})\s*-\s*S(\d{1,3})\s*E(\d{1,4})\b`)
var structureSpaces = strings.NewReplacer(".", " ", "_", " ")

// maxEpisodes bounds the episode sets built from ranges.
const maxEpisodes = 2000

var absoluteEpisode = regexp.MustCompile(`(?i)\s-\s(\d{1,4}(?:\s*-\s*\d{1,4})*)(?:v(\d+))?(?:\s|\[|\(|$)`)
var explicitAbsolute = regexp.MustCompile(`(?i)\b(?:EP|Episode)\s*(\d{1,4})(?:v(\d+))?\b`)
var namedBatch = regexp.MustCompile(`(?i)\b(?:BATCH|COMPLETE)\b`)
var dateEpisode = regexp.MustCompile(`\b((?:19|20)\d{2})[ ._-](\d{2})[ ._-](\d{2})\b`)

func integers(s string, listSeparator string) []int {
	out := []int{}
	for _, part := range strings.Split(strings.ToUpper(strings.ReplaceAll(s, " ", "")), listSeparator) {
		bounds := strings.Split(part, "-")
		first, err := strconv.Atoi(bounds[0])
		if err != nil || first < 0 || first > 9999 {
			return nil
		}
		last := first
		if len(bounds) > 2 {
			return nil
		}
		if len(bounds) == 2 {
			last, err = strconv.Atoi(bounds[1])
			if err != nil || last < first || last > 9999 || last-first > 500 {
				return nil
			}
		}
		if len(out)+last-first >= maxEpisodes {
			return nil
		}
		for n := first; n <= last; n++ {
			out = append(out, n)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}
func numberKey(ns []int) string {
	parts := make([]string, len(ns))
	for i, n := range ns {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ",")
}

func structure(p *models.ParsedRelease, title string, media models.Media, rules models.MatchingRules, evidence *[]string) {
	text := structureSpaces.Replace(title)
	text = repeatedSeason.ReplaceAllStringFunc(text, func(match string) string {
		m := repeatedSeason.FindStringSubmatch(match)
		first, _ := strconv.Atoi(m[1])
		second, _ := strconv.Atoi(m[3])
		if first != second {
			return match
		}
		return "S" + m[1] + m[2] + "-E" + m[4]
	})
	if media.Type != "tv" && !seasonEpisode.MatchString(text) && !xEpisode.MatchString(text) && !seasonPack.MatchString(text) {
		return
	}
	p.MediaType = "tv"
	p.Structure = "unknown"
	p.Season = 0
	p.Episode = 0
	setEpisodes := func(season int, episodes []int) {
		p.Seasons = []int{season}
		p.Season = season
		p.Episodes = episodes
		if len(episodes) > 0 {
			p.Episode = episodes[0]
			p.Structure = fmt.Sprintf("episodes:s%d:e%s", season, numberKey(episodes))
		}
	}
	for _, pattern := range []*regexp.Regexp{seasonEpisode, xEpisode, namedEpisode, animeSeasonEpisode} {
		matches := pattern.FindAllStringSubmatch(text, -1)
		if len(matches) == 0 {
			continue
		}
		season, _ := strconv.Atoi(matches[0][1])
		episodes := []int{}
		for _, m := range matches {
			s, _ := strconv.Atoi(m[1])
			if s != season {
				p.Structure = "unknown:multi_season_episodes"
				return
			}
			parts := integers(strings.ReplaceAll(strings.ToUpper(strings.ReplaceAll(m[2], " ", "")), "-E", "-"), "E")
			if len(parts) == 0 {
				p.Structure = "unknown:episode_range"
				return
			}
			episodes = append(episodes, parts...)
			if len(m) > 3 && m[3] != "" {
				p.Revision = revisionNumber(p.Revision, m[3])
			}
		}
		slices.Sort(episodes)
		setEpisodes(season, slices.Compact(episodes))
		*evidence = append(*evidence, "normalized_episode_set")
		return
	}
	if matches := seasonPack.FindAllStringSubmatch(text, -1); len(matches) > 0 {
		seasons := []int{}
		for _, m := range matches {
			start, end := m[1]+m[3], m[2]+m[4]
			span := start
			if end != "" {
				span += "-" + end
			}
			ns := integers(span, ",")
			if len(ns) == 0 {
				p.Structure = "unknown:season_range"
				return
			}
			seasons = append(seasons, ns...)
		}
		slices.Sort(seasons)
		p.Seasons = slices.Compact(seasons)
		p.Season = p.Seasons[0]
		p.Structure = "seasons:" + numberKey(p.Seasons)
		*evidence = append(*evidence, "normalized_season_pack")
		return
	}
	if m := dateEpisode.FindStringSubmatch(title); len(m) > 0 {
		date := m[1] + "-" + m[2] + "-" + m[3]
		if _, err := time.Parse("2006-01-02", date); err != nil {
			return
		}
		p.Structure = "date:" + date
		return
	}
	var absolute []int
	for _, pattern := range []*regexp.Regexp{absoluteEpisode, explicitAbsolute} {
		if m := pattern.FindStringSubmatch(text); len(m) > 0 {
			absolute = integers(m[1], ",")
			if len(m) > 2 && m[2] != "" {
				p.Revision = revisionNumber(p.Revision, m[2])
			}
			break
		}
	}
	if len(absolute) == 0 {
		return
	}
	p.AbsoluteEpisodes = absolute
	p.Structure = "absolute:" + numberKey(absolute)
	if namedBatch.MatchString(text) {
		p.Structure = "absolute_batch:" + numberKey(absolute)
	}
	*evidence = append(*evidence, "absolute_episode_numbering")
	season := -1
	episodes := []int{}
	mappings := absoluteMappings(rules, media)
	for _, n := range absolute {
		mapping, found := mappings[n]
		if !found {
			return
		}
		if season >= 0 && season != mapping.Season {
			return
		}
		season = mapping.Season
		episodes = append(episodes, mapping.Episode)
	}
	if strings.HasPrefix(p.Structure, "absolute_batch:") {
		return
	}
	slices.Sort(episodes)
	setEpisodes(season, slices.Compact(episodes))
	*evidence = append(*evidence, "explicit_absolute_episode_mapping")
}

// absoluteMappings indexes the episode mappings for media by absolute number.
func absoluteMappings(rules models.MatchingRules, media models.Media) map[int]models.EpisodeMapping {
	key := MediaKey(media)
	out := map[int]models.EpisodeMapping{}
	if key == "" {
		return out
	}
	for _, mapping := range rules.EpisodeMappings {
		if _, seen := out[mapping.Absolute]; !seen && key == MediaKey(models.Media{Type: "tv", IMDbID: mapping.IMDbID, TMDbID: mapping.TMDbID}) {
			out[mapping.Absolute] = mapping
		}
	}
	return out
}

func InScope(p models.ParsedRelease, m models.Media) bool {
	if p.MediaType != m.Type {
		return false
	}
	if m.Season != nil && !slices.Contains(p.Seasons, *m.Season) {
		return false
	}
	if m.Episode != nil && !slices.Contains(p.Episodes, *m.Episode) {
		return false
	}
	return true
}

// InScopeOrPack also accepts, for a single-episode scope, a season pack that
// contains the episode's season: slot and season-pack rules compare the two.
func InScopeOrPack(p models.ParsedRelease, m models.Media) bool {
	if InScope(p, m) {
		return true
	}
	return m.Episode != nil && m.Season != nil && p.MediaType == m.Type && strings.HasPrefix(p.Structure, "seasons:") && slices.Contains(p.Seasons, *m.Season)
}
