package clustering

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"sort"
	"strings"

	"ravenseed/internal/matching"
	"ravenseed/internal/models"
	"ravenseed/internal/releases"
)

func BuildWithRules(results []models.Result, req models.SearchRequest, progress []models.IndexerProgress, rules models.MatchingRules) []models.Family {
	groups := map[string][]models.Result{}
	fingerprints := fingerprintCache{}
	for _, r := range results {
		key := fingerprints.add(r)
		groups[key] = append(groups[key], r)
	}
	groups = mergeSameFiles(groups)
	out := make([]models.Family, 0, len(groups))
	for key, rs := range groups {
		sort.Slice(rs, func(i, j int) bool { return rs[i].ID < rs[j].ID })
		h := sha256.Sum256([]byte(key))
		f := models.Family{ID: hex.EncodeToString(h[:12]), Canonical: rs[0].Parsed, Results: rs, Variants: []models.SizeVariant{}, Confidence: "POSSIBLE", Evidence: []string{"verification_required"}}
		variants := map[string]*models.SizeVariant{}
		all := map[int]bool{}
		for _, r := range rs {
			all[r.IndexerID] = true
			sizeKey := fmt.Sprint(r.Size)
			if r.Size <= 0 {
				sizeKey = r.ID
			}
			v := variants[sizeKey]
			if v == nil {
				v = &models.SizeVariant{Size: r.Size, ResultIDs: []string{}, IndexerIDs: []int{}, Confidence: "POSSIBLE"}
				variants[sizeKey] = v
			}
			v.ResultIDs = append(v.ResultIDs, r.ID)
			if !slices.Contains(v.IndexerIDs, r.IndexerID) {
				v.IndexerIDs = append(v.IndexerIDs, r.IndexerID)
			}
			f.Seeders += max(0, r.Seeders)
			if r.PublishDate.After(f.Newest) {
				f.Newest = r.PublishDate
			}
		}
		for _, v := range variants {
			if v.Size > 0 {
				v.Coverage = len(v.IndexerIDs)
			}
			slices.Sort(v.IndexerIDs)
			if v.Coverage >= 2 {
				complete := map[int]bool{}
				for _, r := range rs {
					if r.Size == v.Size && matching.Complete(r) {
						complete[r.IndexerID] = true
					}
				}
				if len(complete) >= 2 {
					v.Confidence = "EXACT"
				}
			}
			f.Variants = append(f.Variants, *v)
		}
		sort.Slice(f.Variants, func(i, j int) bool {
			a, b := f.Variants[i], f.Variants[j]
			if a.Confidence != b.Confidence {
				return matching.ConfidenceRank(a.Confidence) > matching.ConfidenceRank(b.Confidence)
			}
			if a.Coverage != b.Coverage {
				return a.Coverage > b.Coverage
			}
			if a.Size != b.Size {
				return a.Size > b.Size
			}
			// Unknown-size variants tie on everything else.
			return a.ResultIDs[0] < b.ResultIDs[0]
		})
		primary := f.Variants[0]
		f.Coverage = primary.Coverage
		f.TotalCoverage = len(all)
		var reference models.Result
		for _, r := range rs {
			if slices.Contains(primary.ResultIDs, r.ID) {
				reference = r
				if matching.Complete(r) {
					break
				}
			}
		}
		// Members on the reference's indexer compare with the primary variant's
		// best result from another indexer.
		alternate := reference
		for _, other := range rs {
			if other.IndexerID != reference.IndexerID && slices.Contains(primary.ResultIDs, other.ID) {
				alternate = other
				if matching.Complete(other) {
					break
				}
			}
		}
		for i, r := range f.Results {
			counterpart := reference
			if r.IndexerID == reference.IndexerID {
				counterpart = alternate
			}
			confidence, evidence := compareMember(counterpart, r, rules, fingerprints)
			if r.IndexerID == counterpart.IndexerID {
				confidence = "POSSIBLE"
				evidence = append(evidence, "requires_another_indexer")
			}
			f.Results[i].Confidence = confidence
			f.Results[i].Evidence = evidence
			if r.IndexerID != counterpart.IndexerID && matching.ConfidenceRank(confidence) > matching.ConfidenceRank(f.Confidence) {
				f.Confidence = confidence
				f.Evidence = evidence
			}
		}
		if primary.Confidence == "EXACT" {
			f.Confidence = "EXACT"
			for _, r := range rs {
				if r.IndexerID != reference.IndexerID && r.Size == primary.Size && matching.Complete(r) {
					_, f.Evidence = matching.CompareFingerprinted(reference, r, fingerprints.of(reference), fingerprints.of(r), rules)
					break
				}
			}
			for i := range f.Results {
				if f.Results[i].Size == primary.Size && matching.Complete(f.Results[i]) {
					f.Results[i].Confidence = "EXACT"
					f.Results[i].Evidence = f.Evidence
				}
			}
		}
		for _, v := range []string{f.Canonical.Resolution, f.Canonical.Group, f.Canonical.VideoCodec, f.Canonical.AudioCodec, f.Canonical.AudioChannels, f.Canonical.BitDepth} {
			if v != "" {
				f.Completeness++
			}
		}
		for i := range f.Variants {
			v := &f.Variants[i]
			ref := reference
			for _, result := range rs {
				if slices.Contains(v.ResultIDs, result.ID) {
					ref = result
					if matching.Complete(result) {
						break
					}
				}
			}
			variantFamily := f
			variantFamily.Confidence, variantFamily.Coverage = v.Confidence, v.Coverage
			status := target(variantFamily, ref, results, req, progress, rules, fingerprints)
			v.TargetStatus = &status
		}
		f.TargetStatus = *f.Variants[0].TargetStatus
		for _, v := range f.Variants {
			if v.TargetStatus.Candidate && !f.TargetStatus.Candidate {
				f.TargetStatus.Candidate = true
				f.TargetStatus.Message = "An exact-size variant has no matching result on target"
			}
		}
		out = append(out, f)
	}
	linkRelated(out, rules, fingerprints)
	return out
}

// fingerprintCache holds each result's fingerprint by ID, so comparisons do
// not re-encode the parsed release. IDs seen with two fingerprints are left
// out and computed on demand.
type fingerprintCache map[string]string

func (c fingerprintCache) add(r models.Result) string {
	key := matching.Fingerprint(r.Parsed)
	if previous, seen := c[r.ID]; !seen {
		c[r.ID] = key
	} else if previous != key {
		c[r.ID] = ""
	}
	return key
}

func (c fingerprintCache) of(r models.Result) string {
	if key := c[r.ID]; key != "" {
		return key
	}
	return matching.Fingerprint(r.Parsed)
}

// mergeSameFiles joins name groups holding one file under different tracker
// titles: exact bytes with agreeing release fields, including the group. The
// merged group keeps the smallest key so family IDs do not depend on order.
// Equivalence fills a field named on one side only, so it is not transitive:
// groups join only when every same-size pair across them agrees, which keeps
// AMZN and NF titles apart when a service-less title matches both.
func mergeSameFiles(groups map[string][]models.Result) map[string][]models.Result {
	parent := map[string]string{}
	var find func(string) string
	find = func(k string) string {
		if parent[k] == "" || parent[k] == k {
			return k
		}
		parent[k] = find(parent[k])
		return parent[k]
	}
	type sized struct {
		key string
		r   models.Result
	}
	bySize := map[int64][]sized{}
	for key, rs := range groups {
		for _, r := range rs {
			if r.Size > 0 {
				bySize[r.Size] = append(bySize[r.Size], sized{key, r})
			}
		}
	}
	sizes := make([]int64, 0, len(bySize))
	for size, members := range bySize {
		sizes = append(sizes, size)
		slices.SortFunc(members, func(a, b sized) int { return strings.Compare(a.key+"\x00"+a.r.ID, b.key+"\x00"+b.r.ID) })
	}
	slices.Sort(sizes)
	for _, size := range sizes {
		members := bySize[size]
		for i, a := range members {
			for _, b := range members[i+1:] {
				ka, kb := find(a.key), find(b.key)
				if ka == kb || !matching.ObservedEquivalent(a.r, b.r) {
					continue
				}
				agree := true
				for _, x := range members {
					for _, y := range members {
						if agree && find(x.key) == ka && find(y.key) == kb && !matching.ObservedEquivalent(x.r, y.r) {
							agree = false
						}
					}
				}
				if agree {
					parent[max(ka, kb)] = min(ka, kb)
				}
			}
		}
	}
	merged := map[string][]models.Result{}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		root := find(key)
		merged[root] = append(merged[root], groups[key]...)
	}
	return merged
}

// compareMember rates a family member against its counterpart. Members joined
// as one file under different tracker titles rate on identity and bytes.
func compareMember(a, b models.Result, rules models.MatchingRules, fingerprints fingerprintCache) (string, []string) {
	confidence, evidence := matching.CompareFingerprinted(a, b, fingerprints.of(a), fingerprints.of(b), rules)
	if confidence == "EXACT" || !matching.ObservedEquivalent(a, b) {
		return confidence, evidence
	}
	if matching.Complete(a) && matching.Complete(b) {
		return "EXACT", []string{"same_file_exact_bytes", "tracker_title_differs", "same_group", "same_content_identity"}
	}
	return "POSSIBLE", []string{"same_file_exact_bytes", "tracker_title_differs", "metadata_or_identity_unverified"}
}

func target(f models.Family, reference models.Result, all []models.Result, req models.SearchRequest, progress []models.IndexerProgress, rules models.MatchingRules, fingerprints fingerprintCache) models.TargetStatus {
	t := models.TargetStatus{IndexerID: req.TargetIndexerID, Message: "No target selected"}
	if t.IndexerID == 0 {
		return t
	}
	for _, p := range progress {
		if p.IndexerID == t.IndexerID {
			t.Evaluated = p.Status == "completed"
		}
	}
	if !t.Evaluated {
		t.Message = "Target could not be evaluated"
		return t
	}
	equivalent := false
	referenceKey := fingerprints.of(reference)
	for _, r := range all {
		if r.IndexerID != t.IndexerID {
			continue
		}
		key := fingerprints.of(r)
		c, _ := matching.CompareFingerprinted(reference, r, referenceKey, key, rules)
		// Target titles follow the tracker's naming rules; exact bytes with
		// agreeing release fields are the same file under another name.
		if c == "EXACT" || matching.ObservedEquivalent(reference, r) {
			t.ExactMatchFound = true
			equivalent = true
		} else if c == "POSSIBLE" && (r.Size == reference.Size || r.Size <= 0) {
			t.RelatedMatchFound = true
			equivalent = true
		} else if key == referenceKey {
			t.RelatedMatchFound = true
			if r.Size == reference.Size || r.Size <= 0 {
				equivalent = true
			}
		} else if r.Parsed.MediaType == reference.Parsed.MediaType && ((r.Parsed.ContentID != "" && r.Parsed.ContentID == reference.Parsed.ContentID) || releases.TitleKey(r.Parsed.Title) == releases.TitleKey(reference.Parsed.Title)) && r.Parsed.Structure == reference.Parsed.Structure {
			t.RelatedMatchFound = true
		}
	}
	// f carries its variant's confidence, EXACT or POSSIBLE. STRONG families
	// (sizes within tolerance) are never candidates.
	t.Candidate = !equivalent && f.Confidence == "EXACT" && f.Coverage >= req.Filters.MinimumCoverage
	switch {
	case t.ExactMatchFound:
		t.Message = "Exact reported-size match found on target"
	case equivalent:
		t.Message = "Unverified matching result found on target; verification required"
	case t.RelatedMatchFound:
		t.Message = "No equivalent result found; other releases exist on target"
	default:
		t.Message = "No matching result found on target"
	}
	return t
}

func linkRelated(families []models.Family, rules models.MatchingRules, fingerprints fingerprintCache) {
	byCore := map[string][]int{}
	for i, f := range families {
		p := f.Canonical
		identity := p.ContentID
		if identity == "" {
			identity = releases.TitleKey(p.Title)
		}
		key := fmt.Sprintf("%s|%s|%s|%s|%s|%s", identity, p.Structure, p.Resolution, p.Source, p.ReleaseType, p.Group)
		byCore[key] = append(byCore[key], i)
	}
	for _, ids := range byCore {
		slices.SortFunc(ids, func(a, b int) int { return strings.Compare(families[a].ID, families[b].ID) })
		for a, i := range ids {
			for _, j := range ids[a+1:] {
				if len(families[i].RelatedFamilies) >= 12 && len(families[j].RelatedFamilies) >= 12 {
					continue
				}
				a, b := families[i].Results[0], families[j].Results[0]
				confidence, evidence := matching.CompareFingerprinted(a, b, fingerprints.of(a), fingerprints.of(b), rules)
				if confidence != "POSSIBLE" && confidence != "RELATED" {
					continue
				}
				if len(families[i].RelatedFamilies) < 12 {
					families[i].RelatedFamilies = append(families[i].RelatedFamilies, models.FamilyRelation{FamilyID: families[j].ID, Confidence: confidence, Evidence: evidence})
				}
				if len(families[j].RelatedFamilies) < 12 {
					families[j].RelatedFamilies = append(families[j].RelatedFamilies, models.FamilyRelation{FamilyID: families[i].ID, Confidence: confidence, Evidence: evidence})
				}
			}
		}
	}
}
