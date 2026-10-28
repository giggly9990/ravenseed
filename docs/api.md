# API usage

Interactive docs are served at `/api/docs`. `/api/openapi.json` generates schemas from the Go models and lists all routes. All requests and responses use JSON except SSE. Errors have a consistent shape:

```json
{"error":{"code":"invalid_json","message":"Invalid JSON or unknown fields"}}
```

Status codes: `400` rejected input, `404` unknown record, `409` a conflicting state (a paused monitor, a check already running, an upgrade offer that changed meanwhile), `413` a body over 64 KiB, `429` rate or job limits, `500` a server or database failure (`internal_error` or `database_error`, details only in the log), `502` an upstream service such as qui, TMDb, Sonarr, Radarr or Upload-Assistant failed, `504` timeout.

API clients use `Authorization: Bearer <API_TOKEN>`. Browsers use the server's HttpOnly session cookie after signing in. Omit authentication only when both browser credentials and token/session mode are disabled; then only loopback host names and those in `ALLOWED_HOSTS` are served, and other `Host` values get `403 host_rejected`. Cookie-authenticated mutations require the same-origin Origin header. API documentation is protected when authentication is enabled. See [browser login](browser-login.md).

## Connect qui

```sh
curl http://localhost:8088/api/v1/settings/test \
  -H 'Content-Type: application/json' \
  -d '{"quiUrl":"http://host.docker.internal:7476","apiKey":"YOUR_QUI_KEY"}'

curl -X PUT http://localhost:8088/api/v1/settings \
  -H 'Content-Type: application/json' \
  -d '{"quiUrl":"http://host.docker.internal:7476","apiKey":"YOUR_QUI_KEY","defaultTargetIndexer":3,"defaultMinCoverage":2}'
```

An empty API key preserves the existing key only when the URL is unchanged. Environment-managed connections cannot be replaced by the UI. Neither the test nor settings response returns the key. Indexer responses contain only ID, name, enabled state, backend and capabilities.

## Search

```sh
curl http://localhost:8088/api/v1/searches \
  -H 'Content-Type: application/json' \
  -d '{"media":{"type":"movie","imdbId":"tt0133093"},"indexerIds":[1,2,3],"targetIndexerId":3,"filters":{"minimumCoverage":2}}'
```

At least one identifier is required. When both identifiers are supplied, the metadata resolver must verify their relationship. TMDb IDs are positive integers. IMDb IDs use `tt` followed by 7 to 10 digits. TV searches accept an optional season from 0 to 999 and an optional episode from 1 to 9999. An episode requires a season. An omitted indexer list selects all enabled indexers; an omitted target uses the configured default. The target must belong to the selected set. `refresh:true` requests fresh results. Titles, year and TVDb identity are resolved server-side.

New jobs return HTTP 202 and the job object immediately. A cache hit returns HTTP 200, the previous job ID, and `cacheHit:true`.

```sh
curl http://localhost:8088/api/v1/searches/SEARCH_ID
curl -N http://localhost:8088/api/v1/searches/SEARCH_ID/events
curl -N -H 'Last-Event-ID: 12' http://localhost:8088/api/v1/searches/SEARCH_ID/events
```

SSE events: `search.started`, `indexer.started`, `indexer.completed`, `indexer.failed`, `analysis.started`, `analysis.completed`, `search.completed`, `search.snapshot`. Each data field contains a full job snapshot. Heartbeats are comments. The final snapshot closes the stream. Clients can reconnect with `Last-Event-ID` or poll the job endpoint.

## Results

```sh
curl 'http://localhost:8088/api/v1/searches/SEARCH_ID/families?minimumCoverage=2&confidence=EXACT&sort=coverage&order=desc'
curl 'http://localhost:8088/api/v1/searches/SEARCH_ID/candidates?sort=target&order=asc'
curl http://localhost:8088/api/v1/searches/SEARCH_ID/families/FAMILY_ID
curl 'http://localhost:8088/api/v1/searches/SEARCH_ID/results?limit=50&offset=0'
curl 'http://localhost:8088/api/v1/searches?limit=25&offset=0'
```

Results, families and candidates never contain download links. Each result has `hasDownloadUrl` instead of `downloadUrl`, and `raw` omits link fields and any value carrying a tracker key; the link stays server-side for Add to qui. The same check clears `infoUrl`, on results and on policy target results such as upgrade offers, when it holds a credential, is a download or feed link, or is the download link itself. Keys are matched after HTML unescaping and percent-decoding, so `&amp;passkey=` and `pass%6Bey=` count.

Paginated lists return `{items,total,limit,offset}`. `limit` is 1 to 200 and defaults to 50. Offset must be nonnegative. Indexers are returned as a simple array.

Family filters: `resolution`, `releaseType`, `source`, `codec`, `hdr`, `group`, `confidence`, `minimumCoverage`, `targetMissing`, `exactOnly`. Text filters accept comma-separated values, compared case-insensitively. URL-encode `+` in HDR10+ as `%2B`.

Sort options: `coverage`, `quality`, `seeders`, `size`, `newest`, `target`. Candidates default to target priority; families default to coverage. Minimum coverage defaults to the search request's value. Set it to 0 to inspect unknown-size families.

Raw and analyzed result endpoints are populated after analysis completes. Job snapshots expose live result counts and per-indexer errors during collection. SSE never includes raw download URLs or connection secrets.

The API limits each peer IP to 240 requests per minute. Job saturation returns HTTP 429 with `Retry-After`. Requests larger than 64 KiB (`413`), unknown JSON fields, invalid identifiers, disabled indexers, and target indexers outside the selection are rejected.

## Tracker monitors

Use `GET /api/v1/tracker-hosts` to identify the announce hostname for Tracker A's existing torrents, then `POST /api/v1/monitors` with the source indexer, host, target indexer, interval and the required `instanceIds`, the active, connected qui instances to scan (`GET /api/v1/qui/instances`). Review checks at `/api/v1/monitors/{monitorId}/candidates`; `GET /api/v1/monitor-candidate-counts` totals every monitor's candidates per filter. A confirmed candidate can be queued from its existing qui torrent with `POST /api/v1/monitor-candidates/{candidateId}/upload`. See [tracker monitor setup, status meanings and endpoints](tracker-monitors.md). Configure Sonarr and Radarr with `PUT /api/v1/settings/arr` so monitors can search by ID; see [Sonarr and Radarr IDs](arr-integration.md).

Pause or resume a monitor with `PUT /api/v1/monitors/{monitorId}` and `{ "enabled": false }` or `{ "enabled": true }`. Pausing preserves due checks and candidates. The same request changes the scanned instances with `{ "instanceIds": [7, 8] }`, alone or with `enabled`; `{ "instanceIds": [] }` scans every instance, including ones added to qui later, like monitors created before instance selection. An instance the monitor already covers is kept even while it is offline (for an all-instances monitor, one it has candidates on); an added instance must be active and connected, else the request fails with `400` naming it. qui itself must be reachable (`502` otherwise). An enabled monitor scans the new selection at once. Candidates on removed instances turn **Gone** at their next check and are dropped at the one after; gone candidates on a re-added instance are queued for a fresh check. Saving instances clears the monitor's `lastError`; an enabled monitor whose kept instances are all offline reports that again at once.

`POST /api/v1/monitors/{monitorId}/scan` returns `409` while paused, `400` when the qui connection changed, `502` when qui fails or none of the monitor's instances is active and connected, and `504` on a timeout. `POST /api/v1/monitor-candidates/{candidateId}/upload` returns `400` for a candidate on an instance the monitor no longer scans.

## Matching configuration

`GET /api/v1/settings` includes `matching`. Update it independently with `PUT /api/v1/settings/matching`; this does not test or alter the qui connection. It returns settings without secrets and validates aliases, ignored tags, tolerance and episode mappings before persisting anything.

See [the matching payload and naming rules](advanced-matching.md). `Job.matchingRules` records the effective rules for each new search. `Result.normalizationEvidence` explains applied normalization, and the family comparison evidence explains matching confidence. History from older versions may omit the rule snapshot.

## Tracker flags and resolution

Results expose optional `downloadFactor` and `uploadFactor` values. A reported download factor of 0 means freeleech, 0.5 means half download credit, and an upload factor of 2 means double upload credit. Absent or null values are unknown. Older saved results may contain zero defaults; use the original `raw.download_volume_factor` and `raw.upload_volume_factor` presence when interpreting those records.

Resolution remains a family-list filter, not an upstream search parameter. Changing `GET /searches/SEARCH_ID/families?resolution=1080p` only queries the saved analysis. It never starts another search or discards other resolutions.

## Indexer presets

`GET /api/v1/presets` returns an array of presets. `PUT /api/v1/presets` creates a preset when `id` is omitted, or updates the specified preset:

```json
{"name":"Movie trackers","indexerIds":[1,2,3],"targetIndexerId":3}
```

Names must be unique, nonempty and at most 80 characters. The target must be included in the selected indexers, or 0 for no target. Up to 100 presets are supported. Delete with `DELETE /api/v1/presets/PRESET_ID`. Presets are independent of connection and matching settings.

## Add a selected torrent to qui

`GET /api/v1/qui/instances` returns destination IDs, names, connection state and active state. It omits host credentials. Submit one saved result with:

```sh
curl http://localhost:8088/api/v1/searches/SEARCH_ID/results/RESULT_ID/add \
  -H 'Content-Type: application/json' \
  -d '{"instanceId":7,"category":"Movies","savePath":"/downloads/movies","tags":"ravenseed","paused":true}'
```

Only `instanceId` is required; `paused` defaults to true. The instance must be connected and active. The endpoint resolves the download link and indexer ID from the saved result, then submits a multipart request to qui's `/api/instances/INSTANCE_ID/torrents` endpoint. A successful response must confirm `added:1` and `failed:0`. Success returns `{added:1,failed:0}`. Errors never return upstream response bodies containing tracker links or credentials.

This is a manual torrent add, not payload verification or automatic cross-seed injection. No skip-checking option is sent. Automatic retries are disabled in the UI; after a timeout or uncertain response, check qui before repeating the request. These endpoints use the same authentication and origin checks as other writes.

`GET /api/v1/qui/instances/{instanceId}/categories` returns sorted category names for the selected destination, for example `["Movies","TV"]`. It uses the configured qui connection and the same authentication as other API reads. The selected name is passed as `category` when adding a torrent.

## Media metadata

See [upload workflow API](upload-workflow.md#api) for upload settings, persistent queue history, cancellation and `uploadAfterDownload` torrent options.

`POST /api/v1/media/resolve` accepts `{"id":"tt0133093","type":"movie","refresh":false}`. Type is optional for IMDb lookup. TV requests can include `season`. The response includes resolved IDs, titles, year, overview, artwork paths, optional season details, cache timestamps and any fallback warning. Metadata failures preserve the entered ID for searching.

`PUT /api/v1/settings/metadata` saves `token` and `language` independently of qui settings. A blank token preserves the saved value; `clearToken:true` removes it. Environment-managed tokens cannot be replaced through the API. `POST /api/v1/settings/metadata/test` tests the supplied or saved configuration. Tokens are write-only; settings return `hasToken` and `environmentManaged` instead.

Jobs include a `media` snapshot and a `strategy` with `method` and `query` on each indexer's progress record. Families include optional `relatedFamilies` evidence, and each size variant includes its own `targetStatus`. Family candidate status is true when any qualifying variant is missing from the target.

Family and candidate lists accept `order=asc|desc`, defaulting to `desc`. Direction applies to the full ranking before pagination. Invalid values return HTTP 400.

`GET/PUT /api/v1/settings/torrents` reads or saves `{ "defaultInstanceId": 7, "categories": [{ "instanceId": 7, "enabled": ["Movies"] }] }`. Omit an instance from `categories` to allow all its categories; an empty `enabled` array permits only No category. A missing or zero add-request `instanceId` uses the saved default. Nonempty disabled categories are rejected before adding a torrent.

Upload settings accept `flagPresets: [{ "id": "private", "name": "Private moderation", "args": "--anon --modq" }]` and optional `defaultFlagPresetId`. Upload-enabled add requests accept `uploadPresetId`; omitted uses the default, empty means no preset. Queue jobs expose the snapshotted `flagPresetName` and `args`.
