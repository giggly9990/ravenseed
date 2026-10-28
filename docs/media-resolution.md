# Media resolution

Save a TMDb API key or Read Access Token in Settings > TMDb metadata, or provide `TMDB_TOKEN` or `TMDB_TOKEN_FILE`. Environment credentials take precedence and are not written to SQLite. Saved tokens are never returned by the API. Language defaults to `en-US`.

Standard 32-character hexadecimal API keys use TMDb's `api_key` authentication; Read Access Tokens use Bearer authentication. Both work in the same field. If TMDb returns HTTP 401, verify the credential in your TMDb account's API settings.

Enter an IMDb ID, a numeric TMDb ID, or a prefixed `imdb:` or `tmdb:` ID. Numeric IDs use the selected media type; API resolution defaults to movie when no type is provided. IMDb resolution can infer movie or TV. TV metadata supplies season choices, artwork, synopsis and episode counts. Refresh metadata updates the preview without starting a tracker search.

The resolver maps IMDb, TMDb and available TVDb IDs. Media and season metadata are cached separately in SQLite for seven days. Aliases reuse cached identities. Failed refreshes retain stale metadata with a warning. With no token or no usable metadata, searches retain the entered identifier; metadata failure does not block direct-ID searches. Metadata snapshots stored with completed jobs remain unchanged by later refreshes.

A tracker can retain an older TVDb ID after a series is recreated. For TV results, RavenSeed accepts a differing TVDb ID only when the result itself supplies both the requested IMDb ID and TMDb ID. Search-ID echoes do not qualify. The differing TVDb values are preserved in the original result and explained under its identity evidence. Conflicting IMDb/TMDb IDs, or a TVDb conflict without both corroborating IDs, remain excluded. This does not change which ID is searched: a forced TVDb search can still return nothing when a tracker uses an older ID; select IMDb or TMDb for that tracker.

Search strategy follows the capabilities reported by each indexer:

| Media | Preference |
| --- | --- |
| Movie | IMDb, TMDb, title and year |
| TV | TVDb, IMDb, TMDb, title and year with season or episode |

Unknown capabilities retain optimistic ID searches. Explicitly unsupported searches are reported as such. Title fallback requires a resolved title and query support. Its results need actual matching result IDs for verified identity; echoed search fields alone are insufficient. Search progress records the selected method and query. Upstream failures do not trigger additional search attempts.

Exact-size variants carry independent target status. Missing optional metadata produces explicit POSSIBLE family relationships without combining coverage. Same-size or unknown-size uncertain target matches suppress candidates pending verification. A different-size target listing does not suppress a missing exact-size variant. Metadata confidence does not verify torrent pieces or payload contents.

The red theme, indexer presets, tracker badges, local resolution filtering, episode searches and manual Add to qui flow remain available. Filters support multiple values and codec. Tracker flags remain informational.

The client follows TMDb's [external-ID lookup](https://developer.themoviedb.org/reference/find-by-id), [movie details](https://developer.themoviedb.org/reference/movie-details), [TV details](https://developer.themoviedb.org/reference/tv-series-details) and [season details](https://developer.themoviedb.org/reference/tv-season-details) endpoints. Requests and credentials stay on the server; artwork loads from `image.tmdb.org` in the browser.
