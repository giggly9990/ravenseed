# Verification

How to verify a change locally. CI runs the same Go and frontend checks; see [repository verification](repository-policy.md).

## Unit and integration tests

```sh
test -z "$(gofmt -l cmd internal)"
go vet ./...
go test -race ./...
cd web
npm ci
npm run format:check
npm run typecheck
npm test
npm run build
```

`go test -race` needs cgo; on a machine without a C toolchain, run `go test ./...` locally and rely on CI for race detection. The Docker `test` target runs `go vet` and the race tests without a local Go toolchain: `docker build --target test -t ravenseed-tests .`

Go tests use fixtures and `httptest` servers in place of qui, TMDb, Sonarr, Radarr and Upload-Assistant; they cover upstream failures, partial responses, timeouts, pagination, conflicting media identities, duplicate results, API-key redaction, redirect refusal, unsupported ID capabilities, setup persistence, restart recovery, explicit target selection, the upload queue and duplicate upload reservations. Frontend tests run in jsdom and cover the UI interactions; there are no browser-automation tests, so visual checks are manual.

## Test stack smoke tests

Start the [test stack](../README.md#test-stack) and run the scripts listed there against it. They exercise searches, SSE, caching, matching settings, presets, torrent handoff, the upload queue, tracker monitors, Sonarr and Radarr IDs and upgrades, against mocks only. No real torrent is added and no tracker upload is published.

Also confirm with the test stack that the containers run as UID 10001 with read-only root filesystems, report healthy, and serve the search, settings, history and API documentation pages.

## Upgrades and live services

Before a release, upgrade an existing deployment that keeps its data volume and confirm connection settings, matching rules, presets and search history survive the upgrade and a container restart.

Live qui, TMDb, Sonarr, Radarr and Upload-Assistant checks need the deployment's own credentials and are not part of the automated tests. Prefer read-only checks, such as loading categories or testing a connection; do not add a torrent or run an upload just to verify a build.
