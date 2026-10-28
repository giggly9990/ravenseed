package search

import (
	"context"
	"fmt"
	"ravenseed/internal/qui"
	"slices"
	"strings"
	"time"
)

const siblingIndexMaxAge = 15 * time.Minute

// crossSeedKey identifies torrents that share one payload on disk: the same
// qui instance, content path and exact total size.
func crossSeedKey(instance int, t qui.Torrent) string {
	size := t.TotalSize
	if size <= 0 {
		size = t.Size
	}
	if t.ContentPath == "" || size <= 0 {
		return ""
	}
	return fmt.Sprintf("%d|%d|%s", instance, size, t.ContentPath)
}

// indexCrossSeeds records, for every torrent, the other torrents sharing its
// payload. Sonarr and Radarr only know the hash they downloaded, so a
// cross-seeded copy is identified through its siblings.
func (s *Service) indexCrossSeeds(inventory map[int][]qui.Torrent) {
	groups := map[string][]string{}
	for instance, items := range inventory {
		for _, t := range items {
			if key := crossSeedKey(instance, t); key != "" && torrentHashPattern.MatchString(t.Hash) {
				groups[key] = append(groups[key], strings.ToLower(t.Hash))
			}
		}
	}
	index := map[string][]string{}
	for _, hashes := range groups {
		if len(hashes) < 2 {
			continue
		}
		// A stable order keeps the 20-sibling cap the same between scans.
		slices.Sort(hashes)
		hashes = slices.Compact(hashes)
		for _, h := range hashes {
			for _, other := range hashes {
				if other != h && len(index[h]) < 20 {
					index[h] = append(index[h], other)
				}
			}
		}
	}
	s.siblingMu.Lock()
	s.siblings, s.siblingsAt = index, time.Now()
	s.siblingMu.Unlock()
}

// crossSeeds returns torrents sharing the payload of hash, refreshing a stale
// index from qui. A failed refresh returns what is known.
func (s *Service) crossSeeds(ctx context.Context, c *qui.Client, hash string) []string {
	s.siblingMu.RLock()
	fresh := time.Since(s.siblingsAt) < siblingIndexMaxAge
	s.siblingMu.RUnlock()
	if !fresh {
		if _, err := s.inventory(ctx, c); err != nil {
			s.siblingMu.Lock()
			s.siblingsAt = time.Now() // retry with the next scan, not every check
			s.siblingMu.Unlock()
		}
	}
	s.siblingMu.RLock()
	defer s.siblingMu.RUnlock()
	return append([]string(nil), s.siblings[strings.ToLower(hash)]...)
}
