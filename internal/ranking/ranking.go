package ranking

import (
	"sort"
	"strconv"
	"strings"

	"ravenseed/internal/matching"
	"ravenseed/internal/models"
	"ravenseed/internal/releases"
)

func quality(f models.Family) int {
	n, _ := strconv.Atoi(strings.TrimRight(f.Canonical.Resolution, "pi"))
	return releases.ClassRank(f.Canonical.ReleaseType)*10000 + n
}

// Sort orders families by the named key, then by the default order. The
// default, used for "coverage", keeps confidence above coverage so popular
// unverified families never outrank verified ones.
func Sort(fs []models.Family, by string) {
	sort.SliceStable(fs, func(i, j int) bool {
		a, b := fs[i], fs[j]
		switch by {
		case "quality":
			if quality(a) != quality(b) {
				return quality(a) > quality(b)
			}
		case "seeders":
			if a.Seeders != b.Seeders {
				return a.Seeders > b.Seeders
			}
		case "size":
			if a.Variants[0].Size != b.Variants[0].Size {
				return a.Variants[0].Size > b.Variants[0].Size
			}
		case "newest":
			if !a.Newest.Equal(b.Newest) {
				return a.Newest.After(b.Newest)
			}
		case "target":
			if a.TargetStatus.Candidate != b.TargetStatus.Candidate {
				return a.TargetStatus.Candidate
			}
			if a.Coverage != b.Coverage {
				return a.Coverage > b.Coverage
			}
			if a.TotalCoverage != b.TotalCoverage {
				return a.TotalCoverage > b.TotalCoverage
			}
		}
		if a.Confidence != b.Confidence {
			return matching.ConfidenceRank(a.Confidence) > matching.ConfidenceRank(b.Confidence)
		}
		if a.Coverage != b.Coverage {
			return a.Coverage > b.Coverage
		}
		if a.TotalCoverage != b.TotalCoverage {
			return a.TotalCoverage > b.TotalCoverage
		}
		if a.TargetStatus.Candidate != b.TargetStatus.Candidate {
			return a.TargetStatus.Candidate
		}
		if quality(a) != quality(b) {
			return quality(a) > quality(b)
		}
		if a.Completeness != b.Completeness {
			return a.Completeness > b.Completeness
		}
		if a.Seeders != b.Seeders {
			return a.Seeders > b.Seeders
		}
		return a.ID < b.ID
	})
}
