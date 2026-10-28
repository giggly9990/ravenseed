# Matching rules

The current engine is conservative. All comparisons are scoped to one requested media identity. Result-provided IMDb/TMDb IDs that contradict the requested ID are discarded by the qui adapter. Exact confidence also requires a matching result ID or a matching `search_imdb_id`/`search_tmdb_id` from qui.

## Families

The normalized family key includes verified content identity (or title when alternate-title matching is disabled), year, media type, TV structure, resolution, release type, source, codec, bit depth, HDR, edition, cut, release group, audio, streaming service, languages, correction type, revision and hybrid status. Display group spelling and site tags do not affect the key. Title separators and group casing are normalized. Source and streaming service aliases normalize to explicit canonical values.

Missing metadata is not a wildcard. Different groups remain separate unless an explicit alias joins them.

Tracker titles follow each site's naming rules, so one release can appear as `… S01 KOREAN 1080p …-ARCHIE` on one tracker and `… AKA Gumihodyeon S01 1080p …-ARCHIE` on another. Listings with exactly the same bytes join one family when all release fields named on both sides agree, including the release group: tags named on one side only (language, alternate title, HDR, cut, audio, service, correction) are naming differences. Byte-identical listings under different group tags stay separate. Without identical bytes, the rules below apply. Different release classes, resolution, editions, corrections and TV structures cannot share a family. Bracketed anime groups and absolute episode sets are preserved. Absolute numbers can match each other, but never implicitly match season/episode numbering. Explicit mappings are scoped to one media ID.

## Size variants and confidence

Each positive byte size becomes one variant. Zero and negative sizes each remain separate unknown-size variants with zero coverage. Duplicate listings from one indexer cannot increase coverage. The strongest variant is the family representative, ordered by confidence, unique indexer coverage and size.

| Confidence | Meaning |
| --- | --- |
| `EXACT` | Compatible complete metadata, verified identity and identical positive reported bytes across at least two distinct indexers |
| `STRONG` | Complete matching metadata and identity across distinct indexers, with positive sizes within the configured tolerance (default 0.1 percent) |
| `POSSIBLE` | Incomplete metadata, unknown identity/size, one indexer only, or materially different sizes |
| `RELATED` | Content and structure agree but other metadata differs |
| `REJECTED` | A hard metadata or content mismatch |

The UI compares each result to the representative size. A different-size member is never labeled an exact match to it. Exact variants can still exist elsewhere in the family. A single listing is not cross-indexer evidence.

Families are isolated before pair comparison, so `RELATED` and `REJECTED` are matcher outcomes rather than merged release-family confidence values. The details API retains comparison evidence alongside each result.

## Target analysis

The target must be selected and enabled. A target search must complete without upstream failure, partial coverage, timeout or unsupported ID capability before absence can be evaluated. Even an empty response is not reliable absence if qui marks it partial.

A candidate needs high confidence, the requested minimum exact-size coverage, and no equivalent size variant on the target. Each variant has its own target status. A target member with a different size remains related but does not suppress a missing variant. An uncertain compatible target member with the same or unknown size suppresses a candidate pending verification. Target titles follow each tracker's naming rules, so a target listing with exactly the same bytes counts as an exact match when its stated release fields agree, even if one title adds or drops tags such as a language (KOREAN), an alternate title (AKA …), HDR or cut labels.

Families with missing optional metadata can link as POSSIBLE matches without combining their coverage. Known metadata differences remain RELATED. The details view exposes the evidence and links to the other family. Release checksums are retained as metadata and do not establish payload equivalence.

The message is always about the search result: no matching result was found. The service cannot prove a release is absent from a tracker catalog.

## Ranking

Default order: confidence, representative exact-size coverage, total family indexer coverage, target candidate status, preferred release class, resolution, metadata completeness, seeders, deterministic family ID.

Release-class preference: full disc, remux, encode, WEB-DL, WEBRip, HDTV, other. Target sorting prioritizes candidates, exact-size coverage and total coverage before confidence and quality. Quality, seeders, size and newest sorts are also available.

## Limits

Reported bytes are not torrent payload verification. Rounded tracker sizes, missing IDs or metadata, ambiguous numbering, multilingual editions and absent capability data can reduce confidence or split families. HDR filters label an empty parsed HDR list as SDR for browsing; this is not proof that the payload is SDR. See [advanced matching](advanced-matching.md) for configurable rules, supported naming forms and safe boundaries.

Blu-ray titles using disc codec labels (`HEVC`, `AVC`, `VC-1`, or `MPEG-2`) without an encode or remux tag are classified as full discs by naming convention. Explicit `x264`, `x265`, `AV1`, and `ENCODE` markers retain encode classification; `REMUX` remains separate. This is a title-based inference, not verification of the torrent's file layout.