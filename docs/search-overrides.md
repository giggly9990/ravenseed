# Per-indexer search overrides

Open **Settings > Per-indexer search**, choose a movie and TV search method for each tracker, then choose **Save** in the unsaved-changes bar.

- **Auto** uses the indexer capabilities reported by qui. Movies prefer IMDb, then TMDb; TV prefers TVDb, IMDb, then TMDb. Title search is the fallback when advertised support allows it.
- **IMDb / TMDb / TVDb** forces that ID type even if the indexer reports different support. TVDb is available for TV only.
- **Title** sends the resolved movie or series title instead of an ID. Regular searches include the year and selected season/episode in the query when available.

Overrides select the search method; existing ID mapping still resolves the actual IDs. If the chosen ID is unavailable, RavenSeed uses a known title. If neither is available, that indexer search fails with an explanation. Configure TMDb metadata to resolve missing IDs and titles. An upstream error does not trigger additional searches with other methods.

For forced TV ID searches, unsupported season/episode parameters are omitted and RavenSeed filters the returned releases locally. Identity conflict checks still apply.

Overrides also apply to tracker monitor checks. A qui torrent carries only a name, so a monitor searches by ID only when [Sonarr or Radarr](arr-integration.md) supplied the torrent's IDs; otherwise, or when the ID search finds no equivalent, it searches by title. TV title searches use the broad series title and filter seasons/episodes locally.

Changes apply to new searches and future monitor checks. Running searches and historical findings keep their previous settings/results. Use **Check again** to recheck a monitor item sooner, subject to its normal queue cadence. **Reset all to Auto** stages a reset; save it to apply.

Settings are stored in the existing SQLite data volume and survive restarts. Each override belongs to a qui indexer ID. Revisit them if you replace the qui connection or reuse indexer IDs.

## API

`PUT /api/v1/settings/search-overrides` replaces the override map without modifying connection or metadata settings:

```json
{
  "1": { "movie": "imdb", "tv": "tvdb" },
  "2": { "movie": "title", "tv": "tmdb" }
}
```

Allowed values: `auto`, `imdb`, `tmdb`, `title`, plus `tvdb` for TV. An empty map restores Auto for all indexers. `GET /api/v1/settings` includes `searchOverrides`; `GET /api/v1/indexers` includes each effective `searchOverride` while preserving the advertised capabilities.
