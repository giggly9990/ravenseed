package search

import (
	"context"
	"errors"
	"fmt"
	"ravenseed/internal/database"
	"ravenseed/internal/models"
	"ravenseed/internal/policy"
	"ravenseed/internal/qui"
	"ravenseed/internal/releases"
	"slices"
	"strconv"
	"strings"
)

// Upgrade levels for a source torrent whose slot on the target holds another
// release: a configured rule ranks the target's release higher, or only a
// comparison can tell.
const (
	UpgradeObjective = "upgrade"
	UpgradePossible  = "possible"
)

// upgradeOffer picks the target release that would upgrade the source. A
// release a configured rule ranks higher wins; otherwise a different group's
// release in the same slot is a possible upgrade. Banned groups are never offered.
func upgradeOffer(configured models.IndexerPolicy, source models.Result, e models.PolicyEvaluation) (string, *models.PolicyTargetResult) {
	switch e.Classification {
	case models.PolicyEquivalent, models.PolicyTrump, models.PolicyMissingSlot, models.PolicyCoexistence:
		return "", nil
	}
	allowed := func(t models.PolicyTargetResult) bool {
		return !policy.Banned(configured, models.Result{Title: t.Title, Parsed: models.ParsedRelease{Group: t.Group}})
	}
	slotted := func(t models.PolicyTargetResult) bool {
		return len(t.Upgrade) > 0 && allowed(t) && (t.Relation == "same_slot" || t.Relation == "policy_competitor" || t.Relation == "season_pack")
	}
	for _, t := range e.TargetResults {
		if slotted(t) && t.UpgradeReview == "" {
			offer := t
			return UpgradeObjective, &offer
		}
	}
	// A rule ranks it higher, but the same review checks as a trump apply.
	for _, t := range e.TargetResults {
		if slotted(t) {
			offer := t
			return UpgradePossible, &offer
		}
	}
	for _, t := range e.TargetResults {
		if t.Relation == "same_slot" && allowed(t) && t.Group != "" && !strings.EqualFold(t.Group, source.Parsed.Group) && !clearlyWorse(source, t) {
			offer := t
			return UpgradePossible, &offer
		}
	}
	return "", nil
}

// releaseKey identifies one tracker listing by exact size and title: searches
// return the same torrent with search-specific links and result IDs.
func releaseKey(size int64, title string) string {
	return fmt.Sprintf("%d|%s", size, strings.ToLower(strings.TrimSpace(title)))
}

func upgradeKey(t *models.PolicyTargetResult) string { return releaseKey(t.Size, t.Title) }

// applyUpgrade records the upgrade a completed check found. A dismissal holds
// while the offer stays the same release and lapses when the target changes.
func applyUpgrade(v *models.MonitorCandidate, configured models.IndexerPolicy, source models.Result, e models.PolicyEvaluation) {
	v.Upgrade, v.UpgradeOffer = upgradeOffer(configured, source, e)
	if v.UpgradeOffer != nil && v.UpgradeDismissed != "" && v.UpgradeDismissed != upgradeKey(v.UpgradeOffer) {
		v.UpgradeDismissed = ""
	}
}

// ErrUpgradeChanged reports an upgrade offer replaced while it was dismissed.
var ErrUpgradeChanged = errors.New("the upgrade offer changed; refresh and try again")

// SetUpgradeDismissed hides or restores a candidate's current upgrade offer.
// Only the dismissal changes, and only while the offer is the one read here.
func (s *Service) SetUpgradeDismissed(id string, dismissed bool) (models.MonitorCandidate, error) {
	v, err := s.Store.MonitorCandidate(id)
	if err != nil {
		return v, internal(err)
	}
	if v.UpgradeOffer == nil {
		return v, errors.New("this torrent has no upgrade to dismiss")
	}
	dismissal := ""
	if dismissed {
		dismissal = upgradeKey(v.UpgradeOffer)
	}
	v, err = s.Store.SetUpgradeDismissed(id, dismissal, v.UpgradeOffer.Title, v.UpgradeOffer.Size)
	if errors.Is(err, database.ErrCandidateBusy) {
		return v, ErrUpgradeChanged
	}
	return v, internal(err)
}

// AddUpgrade adds the target's upgrade release to qui. Download links carry
// tracker passkeys and are not stored, so the target is searched again (qui's
// cache allowed) for the listing with the offer's title and exact size.
func (s *Service) AddUpgrade(ctx context.Context, id string, input models.AddTorrentRequest) (models.AddTorrentResponse, error) {
	var out models.AddTorrentResponse
	v, err := s.Store.MonitorCandidate(id)
	if err != nil {
		return out, internal(err)
	}
	if v.UpgradeOffer == nil {
		return out, errors.New("this torrent has no upgrade to add")
	}
	m, err := s.Store.Monitor(v.MonitorID)
	if err != nil {
		return out, internal(err)
	}
	c, settings, err := s.monitorClient()
	if err != nil {
		return out, err
	}
	if settings.QuiURL != m.QuiURL {
		return out, errors.New("qui connection changed; recreate this monitor")
	}
	indexers, err := s.Indexers(ctx, false)
	if err != nil {
		return out, err
	}
	pos := slices.IndexFunc(indexers, func(i models.Indexer) bool { return i.ID == m.TargetIndexerID && i.Enabled })
	if pos < 0 {
		return out, errors.New("target indexer is unavailable or disabled")
	}
	verified := v.Identity != nil && v.Identity.Verified() && v.IdentityError == ""
	want := upgradeKey(v.UpgradeOffer)
	for _, attempt := range monitorAttempts(v.Media, indexers[pos], verified) {
		req := models.SearchRequest{Media: attempt.media, IndexerIDs: []int{m.TargetIndexerID}, TargetIndexerID: m.TargetIndexerID}
		results, err := c.Search(ctx, req, attempt.indexer)
		if err != nil {
			return out, upstream(err)
		}
		for _, r := range results {
			if releaseKey(r.Size, r.Title) == want {
				if out, err = c.AddTorrent(ctx, r, input); errors.Is(err, qui.ErrNoDownloadLink) {
					return out, err
				}
				return out, upstream(err)
			}
		}
	}
	return out, errors.New("the upgrade is no longer listed on the target; check the torrent again")
}

// notablySmaller is the size ratio below which a target release of the same
// class carries clearly fewer bits.
var notablySmaller = map[string]float64{"ENCODE": 0.95, "WEB_DL": 0.90}

// clearlyWorse reports a target release the name and size already rank below
// the source: a notably smaller encode (fewer bits in the same slot), a
// notably smaller WEB-DL (a lower-bitrate stream; small gaps are extra tracks),
// or a lower main audio class. A remux's size reflects its tracks, not quality.
func clearlyWorse(source models.Result, t models.PolicyTargetResult) bool {
	target := models.Result{Title: t.Title, Size: t.Size}
	releases.Parse(&target, models.Media{Type: source.Parsed.MediaType})
	smaller := notablySmaller[source.Parsed.ReleaseType]
	if smaller > 0 && source.Size > 0 && t.Size > 0 && float64(t.Size) < float64(source.Size)*smaller {
		return true
	}
	a, b := source.Parsed, target.Parsed
	if policy.LosslessAudio(a) && !policy.LosslessAudio(b) && b.AudioCodec != "" {
		return true
	}
	ca, cb := channels(a.AudioChannels), channels(b.AudioChannels)
	return ca > 0 && cb > 0 && cb < ca && policy.LosslessAudio(a) == policy.LosslessAudio(b)
}

// channels counts speakers in a layout such as "5.1"; 0 when unknown.
func channels(layout string) int {
	main, lfe, ok := strings.Cut(layout, ".")
	n, err := strconv.Atoi(main)
	if !ok || err != nil {
		return 0
	}
	extra, _ := strconv.Atoi(lfe)
	return n + extra
}
