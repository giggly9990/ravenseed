package policy

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"ravenseed/internal/matching"
	"ravenseed/internal/models"
	"ravenseed/internal/releases"
	"slices"
	"strings"
	"time"
)

var axes = []string{"releaseClass", "resolution", "source", "provider", "codec", "hdr", "cut", "tvStructure", "group", "hybrid"}
var corrections = []string{"REPACK", "PROPER", "RERIP", "REISSUE"}

func Default(indexerID int) models.IndexerPolicy {
	common := models.SlotAxes{ReleaseClass: true, Resolution: true, Codec: true, HDR: true, Cut: true, TVStructure: true}
	web := common
	web.Provider = true
	encode := common
	remux := common
	remux.Codec = false
	full := common
	full.Codec = false
	full.Source = true
	return models.IndexerPolicy{
		IndexerID: indexerID, DetectMissing: true, DetectCoexistence: true, DetectTrump: true, DetectReview: true,
		Slots:          map[string]models.SlotAxes{"WEB_DL": web, "WEBRIP": web, "ENCODE": encode, "REMUX": remux, "FULL_DISC": full, "HDTV": common, "OTHER": common},
		Corrections:    []string{"REPACK", "PROPER", "RERIP"},
		SourcePriority: []string{}, HDRRelations: []models.HDRTrumpRelation{}, ReputableGroups: []string{},
	}
}

func Validate(v models.IndexerPolicy) error {
	if v.IndexerID <= 0 || len(v.Slots) == 0 || v.EncodeSizePercent < 0 || v.EncodeSizePercent > 100 || len(v.HDRRelations) > 30 || len(v.SourcePriority) > 20 || len(v.ReputableGroups) > 100 {
		return errors.New("invalid indexer policy")
	}
	for name, rule := range v.Slots {
		if !slices.Contains(releases.ReleaseClasses, name) || (!rule.ReleaseClass && !rule.Resolution && !rule.Source && !rule.Provider && !rule.Codec && !rule.HDR && !rule.Cut && !rule.TVStructure && !rule.Group && !rule.Hybrid) {
			return errors.New("each release class needs a valid slot axis")
		}
	}
	for _, c := range v.Corrections {
		if !slices.Contains(corrections, c) {
			return fmt.Errorf("unsupported correction %q", c)
		}
	}
	if v.DeadTorrentMaxSeeders != nil {
		return errors.New("seeder-based trumping needs reliable target availability evidence and is not supported yet")
	}
	for _, r := range v.HDRRelations {
		if r.From == "" || r.To == "" || len(r.From) > 40 || len(r.To) > 40 || canonicalHDR(r.From) == canonicalHDR(r.To) {
			return errors.New("invalid HDR trump relation")
		}
	}
	if len(v.EncodeSizeScope) > 10 || v.EncodeSizeSlots < 0 || v.EncodeSizeSlots > 10 || len(v.EncodeBitrateTiers) > 6 {
		return errors.New("invalid encode size settings")
	}
	for _, scope := range v.EncodeSizeScope {
		resolution, codec, ok := strings.Cut(scope, ":")
		if !ok || !slices.Contains(releases.Resolutions, resolution) || codec == "" || len(codec) > 10 {
			return fmt.Errorf("encode size scope %q must look like 1080p:AVC", scope)
		}
	}
	if len(v.BannedGroups) > 500 || len(v.InternalGroups) > 100 || (v.InternalSlots != "" && v.InternalSlots != "separate" && v.InternalSlots != "review") {
		return errors.New("invalid internal group settings")
	}
	// An empty name would match every title ending in "-".
	for _, group := range append(slices.Clone(v.BannedGroups), v.InternalGroups...) {
		if strings.TrimSpace(group) == "" || len(group) > 64 {
			return errors.New("group names must be 1 to 64 characters")
		}
	}
	tiers := map[string]bool{}
	for _, tier := range v.EncodeBitrateTiers {
		if !slices.Contains(releases.Resolutions, tier.Resolution) || tiers[tier.Resolution] || tier.MaxVideoMbps < 1 || tier.MaxVideoMbps > 200 {
			return errors.New("each bitrate tier needs a distinct resolution and a limit from 1 to 200 Mbps")
		}
		tiers[tier.Resolution] = true
	}
	seen := map[string]bool{}
	for _, source := range v.SourcePriority {
		if strings.TrimSpace(source) == "" || len(source) > 40 || seen[canonicalSource(source)] {
			return errors.New("source priority must list each source once")
		}
		seen[canonicalSource(source)] = true
	}
	return nil
}

func slot(p models.ParsedRelease, rules models.SlotAxes) (map[string]string, []models.PolicyEvidence) {
	components := map[string]string{}
	missing := []models.PolicyEvidence{}
	values := map[string]string{
		"releaseClass": p.ReleaseType, "resolution": p.Resolution, "source": p.Source,
		"provider": p.Service, "codec": p.VideoCodec, "hdr": strings.Join(p.HDR, "+"),
		"cut":         strings.Join(append(slices.Clone(p.Cut), p.Edition...), "+"),
		"tvStructure": p.Structure, "group": p.Group,
	}
	if values["hdr"] == "" {
		values["hdr"] = "unmarked"
	}
	if values["cut"] == "" {
		values["cut"] = "standard"
	}
	if values["group"] == "" {
		values["group"] = "nogrp"
	}
	if p.Hybrid {
		values["hybrid"] = "hybrid"
	} else {
		values["hybrid"] = "native"
	}
	selected := map[string]bool{"releaseClass": rules.ReleaseClass, "resolution": rules.Resolution, "source": rules.Source, "provider": rules.Provider, "codec": rules.Codec, "hdr": rules.HDR, "cut": rules.Cut, "tvStructure": rules.TVStructure, "group": rules.Group, "hybrid": rules.Hybrid}
	for _, axis := range axes {
		if !selected[axis] {
			continue
		}
		value := values[axis]
		if axis == "provider" && p.ReleaseType != "WEB_DL" && p.ReleaseType != "WEBRIP" {
			value = "not_web"
		}
		if axis == "tvStructure" && p.MediaType != "tv" {
			value = "movie"
		}
		if value == "" || value == "UNKNOWN" || value == "unknown" {
			missing = append(missing, models.PolicyEvidence{Rule: axis, Detail: "The release has no reliable " + axis + " metadata", Capability: "OBSERVABLE_IF_PRESENT"})
			continue
		}
		components[axis] = strings.ToLower(value)
	}
	return components, missing
}

func slotKey(components map[string]string) string {
	parts := []string{}
	for _, axis := range axes {
		if value, ok := components[axis]; ok {
			parts = append(parts, axis+"="+value)
		}
	}
	return strings.Join(parts, "|")
}

// sameIdentity reports whether two releases cover the same title and, for
// TV, overlapping seasons and episodes, whatever the slot axes select.
func sameIdentity(a, b models.ParsedRelease) bool {
	if !matching.SameContent(a, b) {
		return false
	}
	if a.MediaType != "tv" {
		return true
	}
	if len(a.Seasons) > 0 && len(b.Seasons) > 0 {
		// A multi-season pack's Season is only its first season.
		if !overlap(a.Seasons, b.Seasons) {
			return false
		}
	} else if a.Season > 0 && b.Season > 0 && a.Season != b.Season {
		return false
	}
	if strings.HasPrefix(a.Structure, "episodes:") && strings.HasPrefix(b.Structure, "episodes:") && !overlap(a.Episodes, b.Episodes) {
		return false
	}
	if (strings.HasPrefix(a.Structure, "date:") || strings.HasPrefix(b.Structure, "date:")) && a.Structure != b.Structure {
		return false
	}
	return true
}

func overlap(a, b []int) bool {
	return slices.ContainsFunc(a, func(n int) bool { return slices.Contains(b, n) })
}

// sameExcept reports whether two releases agree on every slot axis the
// policy selects apart from skip, so a cross-slot rule never trades away a
// provider, cut or hybrid difference.
func sameExcept(a, b models.ParsedRelease, rules models.SlotAxes, skip ...string) bool {
	ac, _ := slot(a, rules)
	bc, _ := slot(b, rules)
	for _, axis := range axes {
		if !slices.Contains(skip, axis) && ac[axis] != bc[axis] {
			return false
		}
	}
	return true
}
func correction(p models.ParsedRelease) string {
	if strings.Contains(p.Correction, "REPACK") {
		return "REPACK"
	}
	if strings.Contains(p.Correction, "RERIP") {
		return "RERIP"
	}
	if strings.Contains(p.Correction, "PROPER") {
		return "PROPER"
	}
	if strings.Contains(p.Correction, "REISSUE") {
		return "REISSUE"
	}
	return ""
}

// knownAxisDiffers reports a slot axis both releases state with different values.
func knownAxisDiffers(a, b map[string]string) bool {
	for axis, value := range a {
		if other, ok := b[axis]; ok && other != value {
			return true
		}
	}
	return false
}

func sameGroup(a, b models.ParsedRelease) bool {
	return a.Group != "" && strings.EqualFold(a.Group, b.Group)
}

func compare(a, b models.ParsedRelease, v models.IndexerPolicy, rules models.SlotAxes, sameSlot bool) ([]models.PolicyImprovement, []models.PolicyImprovement) {
	good, bad := []models.PolicyImprovement{}, []models.PolicyImprovement{}
	add := func(kind, from, to string, positive bool) {
		item := models.PolicyImprovement{Type: kind, From: from, To: to}
		if positive {
			good = append(good, item)
		} else {
			bad = append(bad, item)
		}
	}
	if sameSlot {
		ca, cb := correction(a), correction(b)
		// A REPACK, RERIP or REISSUE replaces its own group's release; only a
		// PROPER fixes another group's. A cross-group correction on either
		// side counts as that side's original.
		if !sameGroup(a, b) {
			if ca != "PROPER" {
				ca = ""
			}
			if cb != "PROPER" {
				cb = ""
			}
		}
		if ca != "" && ca == cb && sameGroup(a, b) && a.Revision != b.Revision && a.Revision > 0 && b.Revision > 0 && slices.Contains(v.Corrections, ca) {
			// REPACK2 replaces the same group's REPACK.
			add("correction", fmt.Sprintf("%s%d", cb, b.Revision), fmt.Sprintf("%s%d", ca, a.Revision), a.Revision > b.Revision)
		}
		if ca != cb {
			if ca != "" && cb == "" && slices.Contains(v.Corrections, ca) {
				add("correction", "original", ca, true)
			}
			if cb != "" && ca == "" && slices.Contains(v.Corrections, cb) {
				add("correction", cb, "original", false)
			}
		}
	}
	if v.GroupOverNoGrp && a.ReleaseType == b.ReleaseType && a.Resolution == b.Resolution && a.Source == b.Source && a.VideoCodec == b.VideoCodec && strings.Join(a.HDR, "+") == strings.Join(b.HDR, "+") && strings.Join(a.Cut, "+") == strings.Join(b.Cut, "+") && sameExcept(a, b, rules, "group") {
		trusted := slices.ContainsFunc(v.ReputableGroups, func(g string) bool { return strings.EqualFold(g, a.Group) })
		oldTrusted := slices.ContainsFunc(v.ReputableGroups, func(g string) bool { return strings.EqualFold(g, b.Group) })
		if trusted && b.Group == "" {
			add("group", "NoGrp", a.Group, true)
		}
		if oldTrusted && a.Group == "" {
			add("group", b.Group, "NoGrp", false)
		}
	}
	if v.SeasonPackOverEpisodes && a.MediaType == "tv" && a.Season > 0 && a.Season == b.Season && a.ReleaseType == b.ReleaseType && a.Resolution == b.Resolution && a.Source == b.Source && a.VideoCodec == b.VideoCodec && sameExcept(a, b, rules, "tvStructure") {
		if strings.HasPrefix(a.Structure, "seasons:") && strings.HasPrefix(b.Structure, "episodes:") {
			add("tv_structure", "episode", "season", true)
		}
		if strings.HasPrefix(b.Structure, "seasons:") && strings.HasPrefix(a.Structure, "episodes:") {
			add("tv_structure", "season", "episode", false)
		}
	}
	if v.WebDLOverWebRip && a.Service != "" && a.Service == b.Service && a.Resolution == b.Resolution && a.VideoCodec == b.VideoCodec && strings.Join(a.HDR, "+") == strings.Join(b.HDR, "+") {
		if a.ReleaseType == "WEB_DL" && b.ReleaseType == "WEBRIP" {
			add("web_source", "WEBRIP", "WEB_DL", true)
		}
		if a.ReleaseType == "WEBRIP" && b.ReleaseType == "WEB_DL" {
			add("web_source", "WEB_DL", "WEBRIP", false)
		}
	}
	if a.ReleaseType == b.ReleaseType && a.Resolution == b.Resolution && a.VideoCodec == b.VideoCodec && strings.Join(a.HDR, "+") == strings.Join(b.HDR, "+") && sameExcept(a, b, rules, "source") {
		ai, bi := sourceRank(v, a.Source), sourceRank(v, b.Source)
		if ai >= 0 && bi >= 0 && ai != bi {
			add("source", b.Source, a.Source, ai > bi)
		}
	}
	if a.ReleaseType == b.ReleaseType && a.Resolution == b.Resolution && a.VideoCodec == b.VideoCodec && a.Service == b.Service && a.Source == b.Source && strings.Join(a.Cut, "+") == strings.Join(b.Cut, "+") {
		ah, bh := strings.Join(a.HDR, "+"), strings.Join(b.HDR, "+")
		for _, relation := range v.HDRRelations {
			from, to := canonicalHDR(relation.From), canonicalHDR(relation.To)
			if from == ah && to == bh {
				add("hdr", bh, ah, true)
			}
			if from == bh && to == ah {
				add("hdr", bh, ah, false)
			}
		}
	}
	return good, bad
}

// Evaluate uses already-parsed RavenSeed results. A failed or partial target search
// must be passed as targetComplete=false so it cannot produce a missing candidate.
func Evaluate(v models.IndexerPolicy, candidate models.Result, target []models.Result, targetComplete bool) models.PolicyEvaluation {
	return EvaluateWith(v, candidate, target, targetComplete, Context{})
}

// EvaluateWith also uses title facts, such as the runtime bitrate tiers need.
func EvaluateWith(v models.IndexerPolicy, candidate models.Result, target []models.Result, targetComplete bool, ctx Context) models.PolicyEvaluation {
	out := models.PolicyEvaluation{Classification: models.PolicyUnevaluated, AutomaticEvidence: []models.PolicyEvidence{}, MissingEvidence: []models.PolicyEvidence{}, ReviewReasons: []string{}, CompetingResultIDs: []string{}, TargetResults: []models.PolicyTargetResult{}, Improvements: []models.PolicyImprovement{}, Regressions: []models.PolicyImprovement{}, PolicySnapshotID: v.SnapshotID, EvaluatedAt: time.Now().UTC()}
	if !v.Enabled {
		out.Reason = "Candidate policy is disabled"
		return out
	}
	if !targetComplete {
		out.Reason = "Target search did not complete successfully"
		return out
	}
	p := candidate.Parsed
	// An observed duplicate takes precedence over missing policy metadata and
	// optional quality review. Policy eligibility cannot make it uploadable.
	for _, other := range target {
		if !matching.ObservedEquivalent(candidate, other) {
			continue
		}
		out.Classification, out.Reason = models.PolicyEquivalent, "Matching release and exact reported byte size observed on target"
		out.CompetingResultIDs = append(out.CompetingResultIDs, other.ID)
		out.TargetResults = append(out.TargetResults, models.PolicyTargetResult{ID: other.ID, Title: other.Title, Indexer: other.Indexer, Size: other.Size, InfoURL: other.InfoURL, Relation: "equivalent"})
		out.AutomaticEvidence = append(out.AutomaticEvidence, models.PolicyEvidence{Rule: "observed_equivalent", Detail: "Exact reported bytes and matching release metadata; optional dual-audio labeling does not change presence", Capability: "AUTOMATIC"})
		if slotRules, ok := v.Slots[p.ReleaseType]; ok {
			out.SlotComponents, _ = slot(p, slotRules)
			out.SlotKey = slotKey(out.SlotComponents)
		}
		return out
	}
	if Banned(v, candidate) {
		out.Classification = models.PolicyNotEligible
		out.Reason = "Release group " + p.DisplayGroup + " is banned on this tracker"
		out.AutomaticEvidence = append(out.AutomaticEvidence, models.PolicyEvidence{Rule: "banned_group", Detail: p.DisplayGroup + " is on this tracker's banned list", Capability: "AUTOMATIC"})
		return out
	}
	if p.Title == "" || p.MediaType == "" || candidate.Size <= 0 || (p.MediaType == "movie" && p.Year <= 0) || (p.MediaType == "tv" && p.Season <= 0 && !evaluableTV(p)) {
		out.Reason = "Source release identity or size is incomplete"
		return out
	}
	rules, ok := v.Slots[p.ReleaseType]
	if !ok {
		out.Reason = "No slot rules are configured for this release class"
		return out
	}
	components, missing := slot(p, rules)
	out.SlotComponents, out.SlotKey, out.MissingEvidence = components, slotKey(components), missing
	if len(missing) > 0 {
		out.Reason = "Required slot metadata is unavailable"
		return out
	}
	sourceTier, sourceTierDetail := bitrateTier(v, candidate, ctx, ctx.SourceVideoMbps)
	if sourceTier != "" && sourceTier != "unknown" {
		out.SlotComponents["sizeTier"] = sourceTier
		out.SlotKey += "|sizeTier=" + sourceTier
	}
	// With a slot cap, an encode outside every size boundary competes with the
	// nearest occupant once all size slots are taken.
	sizeScoped := p.ReleaseType == "ENCODE" && v.EncodeSizePercent > 0 && inSizeScope(v, p)
	sizeGap := func(other models.Result) float64 {
		return math.Abs(float64(candidate.Size-other.Size)) / math.Max(float64(candidate.Size), float64(other.Size)) * 100
	}
	nearest := ""
	if sizeScoped && v.EncodeSizeSlots > 0 {
		occupants, within := []models.Result{}, false
		for _, other := range target {
			oc, om := slot(other.Parsed, rules)
			if sameIdentity(p, other.Parsed) && other.Size > 0 && len(om) == 0 && slotKey(oc) == slotKey(components) {
				occupants = append(occupants, other)
				within = within || sizeGap(other) < v.EncodeSizePercent
			}
		}
		if !within && len(occupants) >= v.EncodeSizeSlots {
			nearest = slices.MinFunc(occupants, func(a, b models.Result) int { return cmp.Compare(sizeGap(a), sizeGap(b)) }).ID
		}
	}
	packs := []string{}
	tierUnknown := false
	// blockers counts competitors the candidate does not beat; unknownBlockers
	// those among them whose size tier is unknown.
	blockers, unknownBlockers := 0, 0
	occupants := []models.ParsedRelease{}
	compatible := 0
	coexist := false
	competing := 0
	allImproved := true
	reviewComparison := false
	unmarkedHDR := false
	// uncertain counts listings that may occupy this slot but whose metadata
	// cannot settle it. They never make a candidate; a definite occupant
	// still decides.
	uncertain := 0
	possibleOccupant := func(observed *models.PolicyTargetResult, evidence ...models.PolicyEvidence) {
		uncertain++
		observed.Relation = "uncertain"
		out.MissingEvidence = append(out.MissingEvidence, evidence...)
	}
	for _, other := range target {
		q := other.Parsed
		if !sameIdentity(p, q) {
			continue
		}
		// Without a year, a same-titled listing may be another film, unless
		// matching verified IDs settle it.
		yearUnknown := p.MediaType == "movie" && p.Year > 0 && q.Year <= 0 && (p.ContentID == "" || p.ContentID != q.ContentID)
		compatible++
		out.TargetResults = append(out.TargetResults, models.PolicyTargetResult{ID: other.ID, Title: other.Title, Indexer: other.Indexer, Size: other.Size, InfoURL: other.InfoURL, Relation: "unresolved", Group: q.Group})
		observed := &out.TargetResults[len(out.TargetResults)-1]
		if v.EpisodesBlockedByPack && strings.HasPrefix(p.Structure, "episodes:") && strings.HasPrefix(q.Structure, "seasons:") && slices.Contains(q.Seasons, p.Season) {
			observed.Relation = "season_pack"
			observed.Upgrade, _ = compare(q, p, v, rules, false)
			observed.UpgradeReview = upgradeReview(v, rules, p, q, observed.Upgrade)
			packs = append(packs, other.ID)
			continue
		}
		if p.ReleaseType == "FULL_DISC" && q.ReleaseType == "FULL_DISC" {
			possibleOccupant(observed, models.PolicyEvidence{Rule: "disc_identity", Detail: "Distinct disc pressing or region is not established", Capability: "MANUAL_REVIEW"})
			continue
		}
		otherComponents, otherMissing := slot(q, rules)
		if len(otherMissing) > 0 {
			// A listing whose known metadata differs is in another slot whatever it lacks.
			if knownAxisDiffers(components, otherComponents) {
				observed.Relation = "different_slot"
				coexist = true
			} else {
				possibleOccupant(observed, otherMissing...)
			}
			continue
		}
		same := slotKey(components) == slotKey(otherComponents)
		if same && sizeScoped && other.Size <= 0 {
			possibleOccupant(observed, models.PolicyEvidence{Rule: "encode_size", Detail: "Target encode reported size is unavailable", Capability: "OBSERVABLE_IF_PRESENT"})
			continue
		}
		if same && sizeScoped && sizeGap(other) >= v.EncodeSizePercent && other.ID != nearest {
			same = false
		}
		if same && v.InternalSlots == "separate" && internal(v, p) != internal(v, q) {
			// Internals of this tracker coexist with non-internal releases.
			same = false
			out.AutomaticEvidence = append(out.AutomaticEvidence, models.PolicyEvidence{Rule: "internal_slot", Detail: q.DisplayGroup + " and " + p.DisplayGroup + ": internal and non-internal releases use separate slots on this tracker", Capability: "AUTOMATIC"})
		}
		otherTierUnknown := false
		if same && sourceTier != "" {
			otherTier, otherDetail := bitrateTier(v, other, ctx, 0)
			switch {
			case sourceTier != "unknown" && otherTier != "unknown":
				same = otherTier == sourceTier
			case sizeGap(other) > 5:
				// Competing is the safe assumption; the finding goes to review.
				detail := sourceTierDetail
				if sourceTier != "unknown" {
					detail = otherDetail
				}
				tierUnknown, otherTierUnknown = true, true
				out.MissingEvidence = append(out.MissingEvidence, models.PolicyEvidence{Rule: "encode_tier", Detail: detail, Capability: "OBSERVABLE_IF_PRESENT"})
			}
		}
		good, bad := compare(p, q, v, rules, same)
		if same && Banned(v, other) {
			// The tracker refuses the occupant's group; the candidate, from an
			// allowed group, replaces it in the same slot.
			good = append(good, models.PolicyImprovement{Type: "banned_group", From: q.DisplayGroup + " (banned)", To: p.DisplayGroup})
		}
		if yearUnknown && (same || len(good) > 0 || len(bad) > 0) {
			possibleOccupant(observed, models.PolicyEvidence{Rule: "year", Detail: "Target release year is unavailable", Capability: "OBSERVABLE_IF_PRESENT"})
			continue
		}
		if same || len(good) > 0 || len(bad) > 0 {
			if same {
				observed.Relation = "same_slot"
			} else {
				observed.Relation = "policy_competitor"
			}
			reviewComparison = reviewComparison || (p.Group != "" && q.Group != "" && !strings.EqualFold(p.Group, q.Group)) || (p.Source != q.Source)
			competing++
			unmarkedHDR = unmarkedHDR || (rules.HDR && hdrUncertain(p, q))
			if same {
				occupants = append(occupants, q)
			}
			// The same rules, read from the target's side, say what would
			// upgrade the candidate.
			observed.Upgrade, _ = compare(q, p, v, rules, same)
			observed.UpgradeReview = upgradeReview(v, rules, p, q, observed.Upgrade)
			if len(good) == 0 {
				allImproved = false
			}
			if len(good) == 0 || len(bad) > 0 {
				blockers++
				if otherTierUnknown {
					unknownBlockers++
				}
			}
			out.CompetingResultIDs = append(out.CompetingResultIDs, other.ID)
			out.Improvements = append(out.Improvements, good...)
			for _, improvement := range good {
				out.AutomaticEvidence = append(out.AutomaticEvidence, models.PolicyEvidence{Rule: improvement.Type, Detail: improvement.From + " → " + improvement.To, Capability: "AUTOMATIC"})
			}
			out.Regressions = append(out.Regressions, bad...)
			out.AutomaticEvidence = append(out.AutomaticEvidence, models.PolicyEvidence{Rule: "target_competitor", Detail: "Compared with target result " + other.ID, Capability: "AUTOMATIC"})
		} else {
			observed.Relation = "different_slot"
			coexist = true
		}
	}
	switch {
	case competing > 0 && allImproved && len(out.Improvements) > 0 && len(out.Regressions) == 0 && v.DetectTrump:
		out.Classification, out.Reason = models.PolicyTrump, "Configured objective improvement over observed target slot"
	case competing > 0 && reviewComparison && len(out.Regressions) == 0 && (v.ReviewVisualQuality || v.ReviewSuperiorMaster) && v.DetectReview:
		out.Classification, out.Reason = models.PolicyPossibleTrump, "Different releases compete for this slot; quality comparison requires evidence"
		if v.ReviewVisualQuality {
			out.ReviewReasons = append(out.ReviewReasons, "Visual quality cannot be established from release metadata")
		}
		if v.ReviewSuperiorMaster {
			out.ReviewReasons = append(out.ReviewReasons, "Master quality requires independent evidence")
		}
	case competing > 0:
		out.Classification, out.Reason = models.PolicyNotEligible, "Target slot is occupied without a configured objective improvement"
	case compatible > 0 && coexist && v.DetectCoexistence:
		out.Classification, out.Reason = models.PolicyCoexistence, "Same title has target releases in different policy slots"
	case compatible == 0 && v.DetectMissing:
		out.Classification, out.Reason = models.PolicyMissingSlot, "No matching title or slot occupant observed in completed target search"
	case compatible > 0 && v.DetectMissing:
		out.Classification, out.Reason = models.PolicyMissingSlot, "No occupant observed for this policy slot"
	default:
		out.Classification, out.Reason = models.PolicyNotEligible, "Candidate type is disabled by this policy"
	}
	if len(packs) > 0 {
		out.Classification = models.PolicyNotEligible
		out.Reason = fmt.Sprintf("A season %d pack is on the target; single episodes of that season are not accepted", p.Season)
		out.CompetingResultIDs = append(out.CompetingResultIDs, packs...)
		return out
	}
	if uncertain > 0 && (out.Classification == models.PolicyTrump || out.Classification == models.PolicyMissingSlot || out.Classification == models.PolicyCoexistence) {
		out.Classification = models.PolicyUnevaluated
		out.Reason = "A target release lacks metadata needed to establish its slot"
		return out
	}
	// A definite occupant in a known tier keeps NotEligible; only blockers
	// that may sit in another tier send it to review.
	if tierUnknown && (out.Classification == models.PolicyTrump || (out.Classification == models.PolicyNotEligible && blockers > 0 && unknownBlockers == blockers)) {
		if v.DetectReview {
			out.Classification = models.PolicyPossibleTrump
		} else {
			out.Classification = models.PolicyUnevaluated
		}
		out.Reason = "The encode size tier is unknown; the target encode may be in another tier"
		out.ReviewReasons = append(out.ReviewReasons, "Compare video bitrates against the tracker's size tiers")
		return out
	}
	// Review rules apply only when no regression stands and the rule could
	// beat every occupant.
	if out.Classification == models.PolicyNotEligible && len(occupants) > 0 && len(out.Regressions) == 0 && v.DetectReview {
		reasons := []string{}
		if v.ReviewDualAudio && dualAudio(p) && !slices.ContainsFunc(occupants, dualAudio) {
			reasons = append(reasons, "A dual-audio release may trump one with a single audio language; confirm the existing audio")
		}
		if v.InternalSlots == "review" && internal(v, p) && !slices.ContainsFunc(occupants, func(q models.ParsedRelease) bool { return internal(v, q) }) {
			reasons = append(reasons, "An internal of this tracker may take an additional slot, or trump when equivalent or superior")
		}
		if v.ReviewLosslessWeb && p.ReleaseType == "WEB_DL" && p.Resolution == "2160p" && LosslessAudio(p) && !slices.ContainsFunc(occupants, LosslessAudio) {
			reasons = append(reasons, "A 2160p WEB-DL with lossless audio may take the additional lossless WEB-DL slot")
		}
		if len(reasons) > 0 {
			out.Classification, out.Reason = models.PolicyPossibleTrump, "The slot is occupied, but a rule the release name cannot settle may apply"
			out.ReviewReasons = append(out.ReviewReasons, reasons...)
		}
	}
	if out.Classification == models.PolicyTrump && unmarkedHDR {
		out.MissingEvidence = append(out.MissingEvidence, models.PolicyEvidence{Rule: "hdr", Detail: "One competing release does not state its HDR format", Capability: "OBSERVABLE_IF_PRESENT"})
		out.ReviewReasons = append(out.ReviewReasons, "Confirm both HDR formats before proposing a trump")
		if v.DetectReview {
			out.Classification = models.PolicyPossibleTrump
		} else {
			out.Classification = models.PolicyUnevaluated
		}
		out.Reason = "HDR identity is unmarked on a competing release"
	}
	if out.Classification == models.PolicyTrump && v.ReviewHDRComparability && slices.ContainsFunc(out.Improvements, func(item models.PolicyImprovement) bool { return item.Type == "hdr" }) {
		out.MissingEvidence = append(out.MissingEvidence, models.PolicyEvidence{Rule: "hdr_comparability", Detail: "Comparable video quality is not established by release metadata", Capability: "MANUAL_REVIEW"})
		out.ReviewReasons = append(out.ReviewReasons, "Confirm comparable video quality before proposing an HDR trump")
		if v.DetectReview {
			out.Classification = models.PolicyPossibleTrump
		} else {
			out.Classification = models.PolicyUnevaluated
		}
		out.Reason = "HDR ranking requires comparable video quality"
	}
	return out
}
