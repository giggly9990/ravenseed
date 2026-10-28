# Advanced matching

RavenSeed 0.2.0 implements phase 8 through explicit normalization and comparison rules. Configure them under **Settings > Matching rules** or through `PUT /api/v1/settings/matching`. Connection credentials are not required to save matching rules.

## Alternate titles

Alternate-title matching is enabled by default. Releases with different parsed titles can share a family only when each has evidence for the same requested IMDb or TMDb identity. A contradictory result ID is rejected by the qui adapter. A result without identity evidence cannot join a verified alternate-title family.

Year, edition, cut, release group and technical metadata still participate in the family key. This does not use fuzzy title similarity, guess translations or resolve external metadata. Turn the rule off to require normalized parsed titles to agree.

## Corrections and hybrid releases

`REPACK`, `RERIP` and `PROPER` remain distinct correction types. Numeric revisions normalize across `REPACK2` and `REPACK.2`. The unnumbered correction is revision 1. Anime `v2` and numbered scene episodes preserve their revision. Corrections never merge into the original release, different correction types remain separate, and revisions 1 and 2 cannot merge. Invalid or unsupported revisions cannot receive high confidence.

Hybrid releases only match other hybrid releases with compatible source, group, codec, HDR, audio, edition and correction metadata. The hybrid tag does not relax source or HDR checks. A hybrid DV remux cannot become equivalent to a non-hybrid remux merely because their reported sizes agree.

## WEB and site normalization

`WEB-DL`, `WEBDL`, `WEB DL` and `WEB.DL` normalize to the same source/type. `WEBRip` remains a different release type.

Streaming service aliases include Netflix/NF, Amazon/AMZN, Disney+/DSNP, HBO Max/MAX/HMAX, Apple TV+/ATV/ATVP, Paramount+/PMTP, Peacock/PCOK and Crunchyroll/CR. Different services stay separate. Unknown service tags are preserved rather than guessed.

Domain-shaped tracker prefixes and suffixes such as `[tracker.example]`, and `-GROUP@tracker.example`, do not change release-group identity. The site remains available in parsed metadata. Bracketed anime groups retain their display spelling. Trailing eight-character hexadecimal anime checksums are excluded from matching but remain in the original title and parser output.

## Seasons and episodes

| Naming forms | Normalized meaning |
| --- | --- |
| `S01E02`, `1x02`, `Season 1 Episode 2` | Season 1, episode 2 |
| `S01E01E02`, `S01E01-E02`, `S01E01-02` | Season 1, episodes 1 and 2 |
| `S01E01.S01E02` | Season 1, episodes 1 and 2 |
| `S01`, `Season 1` | Season 1 pack |
| `S01-S03`, `S01.S02.S03`, `Seasons 1-3` | Seasons 1 through 3 |
| `Title - 13`, `Title EP13` | Absolute episode 13 |
| `Title - 01-12 BATCH` | Absolute batch 1 through 12 |
| `2026.09.22` | Date-based episode |

Single episodes, episode sets, season packs and absolute batches are separate structures. A season pack is never assumed to be one episode, and an absolute batch is never assumed to be a complete season. Ambiguous cross-season episode sets, descending ranges and invalid dates cannot receive high confidence.

TV searches support optional `season` and `episode`; an episode requires a season. A season search includes multi-season packs containing that season. An episode search includes episode sets containing the requested episode and excludes season packs. Known indexer capabilities must support the requested TV parameters; unsupported searches are failures, not evidence that the target lacks a release.

Absolute mappings are explicit and identity-scoped. For example, `tmdb:1399 13 = S02E01` lets absolute episode 13 match season 2 episode 1 for searches using that TMDb ID. Unmapped numbers and mappings for a different ID do not cross numbering systems. Every member of an absolute episode range must map to the same season before the range can be compared with a season/episode set.

## Group aliases and ignored tags

Group aliases are case-insensitive. Chains are flattened to their final canonical group. Self-aliases, cycles and conflicting case variants are rejected. Display spelling is retained separately. Explicit aliases are the only way to join different release-group names.

Ignored tags are literal cosmetic labels, matched at token or bracket boundaries. They do not remove text from inside a word or a hyphenated release-group suffix. A leading bracket that identifies a release group is retained. Quality, source, correction, HDR, codec and episode tags cannot be configured as ignored tags. There is no arbitrary regular-expression replacement.

## Size tolerance

The tolerance is a percentage from 0 to 1, with a default of 0.1. Zero disables close-size matching. The comparison uses `abs(a-b) / max(a,b) * 100`. Sizes must be positive and core metadata complete before a close-size comparison can be `STRONG`.

Tolerance never combines byte-size variants or expands exact-size coverage. Only identical positive bytes can produce an `EXACT` comparison. Near-size results still require payload verification.

## API example

```json
{
  "matchAlternateTitles": true,
  "sizeTolerancePercent": 0.1,
  "groupAliases": {"OLDGROUP": "CANONICALGROUP"},
  "ignoredTags": ["CUSTOMTAG"],
  "episodeMappings": [
    {"tmdbId": 1399, "absolute": 13, "season": 2, "episode": 1}
  ]
}
```

The endpoint replaces the matching configuration. Sending empty maps/lists clears those rules. Omitting `matching` when updating connection settings preserves the existing rules. Limits are 100 group aliases, 100 ignored tags and 1,000 episode mappings.

Each job persists its effective `matchingRules`. Rules and engine version participate in the cache key. Changes affect new searches and refreshes; history is never silently reanalyzed. Each result retains `normalizationEvidence` alongside comparison evidence, the original title, original qui response and original parser output.
