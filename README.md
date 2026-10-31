# RavenSeed

RavenSeed is a local web app for finding and comparing releases across the trackers connected to qui. Search for a movie or TV show, choose the trackers to check, and pick a target tracker. RavenSeed helps you see what is already there and which releases may be worth reviewing for upload. It is packaged for both Docker and native installation.

Its features include:

- **Search by media ID:** Enter an IMDb or TMDb ID. With metadata configured, RavenSeed connects matching IMDb, TMDb, and available TVDb IDs, then uses the ID each indexer supports.
- **Compare releases:** View release details, reported sizes, tracker availability, and how confidently listings match, even when verified releases use different titles.
- **Find possible upload candidates:** See releases that were not found on your chosen target tracker, with uncertain matches clearly marked for review.
- **Filter and save searches:** Narrow results by details such as resolution and codec, save tracker selections, and revisit past searches.
- **Monitor existing torrents:** Check torrents already in qui from one tracker against another on a schedule. Pause, resume, or request a fresh check.
- **Identify torrents with Sonarr and Radarr:** Connect your instances so monitors know exactly which movie or episode each torrent is, including cross-seeds and torrents you no longer have, and can recognize listings with different titles or languages. RavenSeed only reads from Sonarr and Radarr and never changes them.
- **Apply tracker rules:** Identify open release slots, versions that may coexist, and possible replacements, including banned groups and size or bitrate tiers. See the target releases behind each finding.
- **Spot upgrades:** See which torrents you seed have been replaced on the target tracker by a better release, such as a REPACK or a season pack. Each one shows why, and you can add the new release to qui or dismiss it.
- **Add releases to qui:** Choose a listing, destination, and optional category.
- **Use Upload-Assistant:** Send an eligible candidate or queue an upload after a new torrent finishes downloading. RavenSeed spaces out submissions and shows each job's outcome.
- **Automate with the API:** Everything in the web app is available through a REST API with an OpenAPI description and interactive docs at `/api/docs`. Use it from scripts or other tools, with token authentication for remote access. Search progress streams live as server-sent events.

RavenSeed presents seed for your review based on your taste. It does not upload a release just because a search or monitor marked it as a candidate.

![Search results comparing release families across three trackers, with the target tracker's status for each](docs/screenshots/search.png)

![Tracker monitor listing torrents from one tracker with their status on the target: missing slot, coexistence, equivalent, occupied or needs review](docs/screenshots/monitors.png)

![Upgrades page listing seeded torrents replaced on the target by a season pack, a REPACK or another group's release](docs/screenshots/upgrades.png)

![Indexer policy settings with slot traits per release class and trump rules](docs/screenshots/indexer-policies.png)

The screenshots use the mock data from the [test stack](#test-stack); no real tracker is shown.

## Install

Each [release](https://github.com/giggly9990/ravenseed/releases) publishes a multi-arch image and native packages for amd64 and arm64.

### Docker

```sh
git clone https://github.com/giggly9990/ravenseed.git
cd ravenseed
docker compose up -d --build
```

To use the published image instead of building, set `image: ghcr.io/giggly9990/ravenseed:<version>` in a Compose override and drop `build`:

```sh
docker pull ghcr.io/giggly9990/ravenseed:latest
```

### Linux packages

Install the `.deb`, `.rpm` or `.apk` for your distribution, edit `/etc/ravenseed/ravenseed.env`, then run `systemctl enable --now ravenseed`. Data lives in `/var/lib/ravenseed`, and the service listens on `127.0.0.1:8080`.

### Archives (Linux, macOS, Windows)

Extract the archive and run `ravenseed` (`ravenseed.exe` on Windows). It serves the bundled `web/` folder next to the binary and stores data in `data/` under the working directory. Configure it with the variables in `.env.example`.

`ravenseed version` prints the build version.

## Connect qui

Open [http://localhost:8088](http://localhost:8088) (Docker) or [http://localhost:8080](http://localhost:8080) (packages and archives), choose **Connect qui**, and enter the qui URL and API key.

- For qui running on the Docker host, use `http://host.docker.internal:7476`.
- For qui on another host, use its reachable HTTP or HTTPS URL.
- For qui on a shared Docker network, see [Shared qui network](#shared-qui-network).
- Include any configured qui base path in the URL. Do not append `/api`.
- Settings warns when a plain `http://` URL for qui, Upload-Assistant, Sonarr or Radarr points outside your local network, because its API key would travel unencrypted. Loopback, private IPs, `.lan`/`.local`/`.internal` names and single-label Docker names are exempt.

In Settings > TMDb metadata, save a TMDb API key or Read Access Token to resolve IDs, show artwork and select TV seasons. Direct-ID searches still work without metadata. See [media resolution](docs/media-resolution.md).

The Docker stack has one non-root container, a read-only root filesystem, and a named SQLite volume. Setup can be completed without editing files. The API key is never returned after saving. A key entered through setup is stored in the private SQLite volume; use Docker secrets if you do not want the key stored there. Search results can contain sensitive download URLs, so protect database backups too.

Compose publishes on `127.0.0.1` by default. Direct binary execution also defaults to loopback. Non-loopback unauthenticated exposure refuses startup unless explicitly allowed. Without authentication, only loopback host names such as `localhost` are served; to open the UI by a LAN name or IP after changing `BIND_ADDRESS`, list it in `ALLOWED_HOSTS`. For remote access, enable [browser login](docs/browser-login.md) (`AUTH_MODE=session`) and use TLS at a reverse proxy; an API token authenticates automation only and does not unlock the WebUI. See [deployment and data protection](docs/security.md) for custom Docker deployments and authenticated proxies.

## Documentation

- [Tracker monitors](docs/tracker-monitors.md) and [upgrades](docs/upgrades.md)
- [Sonarr and Radarr IDs](docs/arr-integration.md)
- [Indexer Policies](docs/indexer-policies.md)
- [Per-indexer search overrides](docs/search-overrides.md)
- [Matching rules](docs/matching.md) and [advanced matching](docs/advanced-matching.md)
- [Upload-Assistant workflow](docs/upload-workflow.md)
- [Browser login](docs/browser-login.md) and [deployment and data protection](docs/security.md)
- [API usage](docs/api.md) and [architecture](docs/architecture.md)

## Environment configuration

Copy `.env.example` to `.env` to override defaults. Environment values take precedence over saved connection settings.

| Variable | Default | Purpose |
| --- | --- | --- |
| `APP_PORT` | `8088` | Host port |
| `BIND_ADDRESS` | `127.0.0.1` | Host bind address |
| `QUI_URL` | empty | Existing qui base URL |
| `QUI_API_KEY` | empty | qui API key |
| `TMDB_TOKEN` | empty | TMDb API key or Read Access Token; also supports `TMDB_TOKEN_FILE` |
| `AUTH_MODE` | empty | `none`, `session` or `token`; empty means `none`, or `token` when `API_TOKEN` is set |
| `API_TOKEN` | empty | Bearer token of at least 24 characters; enables token mode when `AUTH_MODE` is empty; fails with an explicit `AUTH_MODE=none` |
| `WEB_USERNAME` | empty | Single administrator username |
| `WEB_PASSWORD` | empty | Administrator password; prefer `WEB_PASSWORD_FILE` |
| `WEB_PASSWORD_HASH` | empty | Alternative PBKDF2 hash; supports `WEB_PASSWORD_HASH_FILE` |
| `WEB_COOKIE_SECURE` | `true` | Require HTTPS cookies; `false` allowed only for loopback deployments |
| `ALLOW_UNAUTHENTICATED_REMOTE` | `false` | Explicitly permit remote exposure without RavenSeed token authentication |
| `TRUSTED_PROXIES` | empty | Comma-separated proxy IPs/CIDRs permitted to supply client addresses |
| `ALLOWED_HOSTS` | empty | Comma-separated host names or IPs served without authentication, besides loopback |
| `DEFAULT_TARGET_INDEXER` | `0` | No default target when zero |
| `DEFAULT_MIN_COVERAGE` | `2` | Minimum exact-size indexer coverage, from 1 to 1000 |
| `SEARCH_TIMEOUT` | `3m` | Entire job timeout, including queue time |
| `MAX_CONCURRENT_SEARCHES` | `4` | Global concurrent indexer requests |
| `MAX_ACTIVE_JOBS` | `8` | Maximum simultaneous jobs |
| `MAX_CONCURRENT_UPLOADS` | `1` | Concurrent Upload-Assistant executions, from 1 to 4 |
| `UA_SUBMISSION_INTERVAL` | `1m` | Minimum time between Upload-Assistant submissions |
| `HISTORY_RETENTION` | `0` | Search history and ended upload job age limit; `0` disables pruning |
| `RAW_RESULT_RETENTION` | `0` | Raw tracker JSON age limit; normalized results and download URLs remain |
| `RESULT_CACHE_TTL` | `24h` | Analysis cache lifetime; `0s` disables it |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error` |
| `TZ` | `UTC` | Container time zone |

The binary also accepts `DATABASE_PATH`, `LISTEN_ADDR`, `LISTEN_PORT`, and `WEB_DIR`. Compose sets these to container paths and port 8080, and sets `PUBLISHED_BIND_ADDRESS` to the host `BIND_ADDRESS` for startup exposure validation. `TMDB_BASE_URL` (default `https://api.themoviedb.org/3`) changes the TMDb API address.

To use Docker secrets, add a Compose override that mounts a secret and sets `QUI_API_KEY_FILE=/run/secrets/qui_api_key` or `API_TOKEN_FILE=/run/secrets/api_token`. File values take precedence over the corresponding environment value. Use a secret file readable by UID 10001.

```yaml
services:
  ravenseed:
    environment:
      QUI_API_KEY_FILE: /run/secrets/qui_api_key
    secrets:
      - qui_api_key
secrets:
  qui_api_key:
    file: ./secrets/qui_api_key
```

For browser login, configure `WEB_USERNAME` and a web password or secret file. The server protects app pages, assets, docs and APIs before rendering the WebUI. Browser sessions use HttpOnly cookies and can be revoked with Sign out. Automation uses a separate `Authorization: Bearer <API_TOKEN>` credential. Token-only deployments must configure web credentials to use the browser. See [browser login setup](docs/browser-login.md), including the localhost HTTP cookie setting and HTTPS proxy requirements.

`GET /api/v1/health` checks only the process and SQLite, returns `503` if SQLite is unavailable, and never contacts qui. Docker uses this public endpoint. `GET /api/v1/ready` checks qui through the normal authenticated, rate-limited API. It returns `503` when SQLite or the configured qui connection is unavailable. Before qui is configured it returns `200` with `qui: "not_configured"`, allowing initial setup. The WebUI uses readiness for its connection indicator.

## Shared qui network

Set `QUI_NETWORK` to the existing Docker network name and supply the qui API key in `.env` or a secret:

```sh
docker compose -f docker-compose.yml -f docker-compose.example.yml up -d --build
```

The override uses `http://qui:7476`. Change the hostname if qui has a different network alias. The default Compose file does not require an external network.

## Matching behavior

- Preserve original qui results and original `autobrr/rls` parser output.
- Group compatible metadata before splitting by exact positive byte size.
- Count unique indexers, never duplicate listings from one indexer.
- Keep different resolutions, release types, groups, editions and TV structures apart.
- Require verified media identity and complete core metadata for high confidence.
- Treat a failed, partial, timed-out or unsupported target search as unevaluated.
- Evaluate target presence for each exact-size variant. An uncertain same-size target match suppresses its candidate; a different size remains visible as related.
- Order results by confidence, exact-size coverage, total coverage, target status, quality, metadata completeness and seeders.

An `EXACT` label describes reported metadata and bytes. It does not verify torrent hashes, file layout or payload contents. Older qui responses without result IDs or the `search_*` identity fields are retained but cannot receive high confidence.

[Matching rules](docs/matching.md) describe the evidence model and conservative limits. [Advanced matching](docs/advanced-matching.md) covers verified alternate titles, correction revisions, hybrid releases, WEB aliases, anime numbering, TV episode sets, size tolerance, ignored tags and group aliases. Configure these under Settings > Matching rules.

## Search controls and torrent actions

Settings > Torrent defaults lets you choose a default qui destination and enabled categories per instance. Settings > Upload-Assistant supports named free-text UA flag presets, with an optional default and a per-upload picker. Example flags: `--anon --modq`.

Click outside a picker or press Escape to close it. Multiple selections stay open while choosing values. Results support ascending and descending sorting across the full saved result set before pagination, without starting another search.

Choose a resolution before searching. The selection filters the view; RavenSeed still collects and retains every returned resolution. Changing it in the search form or results filters the same saved search without contacting trackers. The choice is included in the page URL and survives reloads.

The media field accepts IMDb IDs, numeric TMDb IDs, and `imdb:` or `tmdb:` prefixes. Resolved metadata provides titles, artwork and TV season choices. Results support multiple values per filter, including codec. Search progress shows the query strategy used for each indexer.

Save named indexer presets from the search form. Each preset includes the selected indexers and target, and can be updated or deleted. Presets persist in SQLite; the last used preset is restored on the same browser. Disabled or removed indexers are omitted with a notice.

Tracker badges show reported freeleech, download discounts and upload multipliers on family cards and individual listings. Missing factors remain unknown. If listings on one tracker have different flags, the family card shows **Mixed flags**. These flags do not affect matching or filtering.

Open **Compare & details**, then choose **Add to qui** on the desired listing. Select a connected qBittorrent instance and optional category, save path and tags. **Add paused** is enabled by default. RavenSeed forwards the saved download URL and indexer ID to qui, which fetches and adds the torrent. It does not skip file checking, map files, or automatically add any torrent. An uncertain response is not automatically retried; check qui before sending it again.

The category picker loads categories from the selected destination. Switching instances clears the selection. Choose **No category** to leave it unset; if categories cannot be loaded, the field accepts manual entry and offers a retry.

## Upload after download

Configure Settings > Upload-Assistant with its WebUI URL, API token, target tracker mappings and download path mappings. Add to qui then offers **Upload to target after download**. The persistent Upload queue waits for all files and invokes only the selected target. See [setup and queue behavior](docs/upload-workflow.md).

## API

- Interactive documentation: [http://localhost:8088/api/docs](http://localhost:8088/api/docs)
- Generated OpenAPI: [http://localhost:8088/api/openapi.json](http://localhost:8088/api/openapi.json)
- [API usage](docs/api.md)
- [Architecture](docs/architecture.md)

The UI uses the same public endpoints as external clients. Lists of jobs, results, families and candidates are paginated. Events support `Last-Event-ID` replay. Failed jobs and partial results remain in history; only fully successful searches are reused from cache.

## Operations

```sh
docker compose ps
docker compose logs --tail=100 ravenseed
docker compose restart ravenseed
docker compose down
```

`down` preserves the named data volume. Back up the volume with the app stopped, or use SQLite's backup API. Do not copy only the database file while WAL writes are active. Interrupted searches are marked failed after restart and can be refreshed. History is retained by default. Optional history and raw-result retention run on startup and hourly; see [retention and backup limits](docs/security.md#stored-data-and-retention).

For production, a suggested starting policy is 30 days of search history and seven days of raw tracker responses. Set these in `.env` when that retention suits your workflow:

```dotenv
HISTORY_RETENTION=720h
RAW_RESULT_RETENTION=168h
```

Defaults remain `0` so upgrades preserve existing data. History retention also deletes finished, failed and cancelled upload jobs of the same age. Active or needs-review uploads protect their search history. Download URLs remain sensitive until the corresponding history is removed, and pruning does not erase earlier backups.

## Development and verification

Requirements: Go 1.26 and Node 26. Docker can build everything without either tool installed locally.

```sh
go test ./...
go vet ./...
cd web
npm ci
npm test
npm run build
```

Start the backend with `go run ./cmd/server`. Start the frontend with `npm run dev` from `web`; it listens on localhost only. `npm run dev:lan` also listens on your network; the proxy forwards the browser's host name, so start the backend with that name or IP in `ALLOWED_HOSTS`. This exposes the API without authentication, so use it only on a trusted network. Vite proxies `/api` to port 8080. `npm run lint` checks the rules of hooks, and `npm run format:check` covers `web/src` and both script folders. The backend serves the production UI from `web/dist` by default.

Docker test target:

```sh
docker build --target test -t ravenseed-tests .
```

### Test stack

`make smoke` starts an isolated stack on `http://localhost:8090` with mock qui, TMDb, Sonarr, Radarr and Upload-Assistant services and its own volume, seeds it and runs the search smoke test. CI runs the same steps. `make test-stack-down` removes it with its data. Its indexers are named Demo Atlas, Demo Orbit and Demo Beacon; nothing reaches a real tracker.

The scripts in `scripts/` exercise the HTTP workflows and take the base URL as their first argument. The test-stack scripts check for its mock qBittorrent instance and change settings, so run them only against the test stack; `auth-smoke.mjs` accepts only a localhost URL. Apart from `MOCK_QUI_URL` below, no script reads environment variables.

| Script | Default URL | Covers |
| --- | --- | --- |
| `node scripts/demo-seed.mjs` | `http://localhost:8090` | Configures mock Sonarr and Radarr, a Demo Beacon policy and a Demo Atlas monitor, and turns Upload-Assistant off. Run it first. `MOCK_QUI_URL` (default `http://mock-qui:7476`) is the mocks' address as RavenSeed reaches them. |
| `node scripts/smoke.mjs` | `http://localhost:8090` | Movie, TV and episode searches, cache reuse, target selection, variants, filters, pagination, raw results, SSE, served assets, matching settings, historical rule snapshots, alternate titles, group aliases, anime mappings, tracker flags, presets and a mock torrent handoff. The handoff does not download or add a real torrent. |
| `node scripts/upload-smoke.mjs` | `http://localhost:8090` | Upload-Assistant handoff, path mapping, flag presets and reupload against the mocks. It changes torrent defaults and Upload-Assistant settings and restores them afterwards, except that an Upload-Assistant token saved for another URL is cleared, with a warning. It passes once per test-stack volume. |
| `node scripts/monitor-smoke.mjs` | `http://localhost:8090` | Tracker monitor results, Sonarr and Radarr IDs, upgrades and arr connection tests. Waits until every mock torrent is checked, about eight minutes on a fresh stack. |
| `node scripts/auth-smoke.mjs [url] [password-file] [username]` | `http://localhost:8088`, `secrets/web_password`, `admin` | Browser login on a local instance with web credentials configured: protected pages and APIs, login, origin checks and logout. |

Unit and integration tests cover parsing, hard mismatches, exact-size variants, duplicate coverage, target failures, timeouts, partial responses, identity evidence, TV season forwarding, cache refresh, durable SSE, authentication, validation, and restart recovery. CI additionally runs race detection and builds and scans the image. Pushing a `v*` tag publishes scanned `linux/amd64` and `linux/arm64` images to GHCR and a GitHub release with archives and packages built by [GoReleaser](.goreleaser.yaml). Run `goreleaser release --snapshot --clean` to build them locally into `.release/`.

See [repository verification](docs/repository-policy.md) for CI enablement and the required branch protection policy.

## License

RavenSeed is licensed under the [GNU Affero General Public License v3.0 or later](LICENSE). If you run a modified version for other people, including over a network, you must make its source available to them.
