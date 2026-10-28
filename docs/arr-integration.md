# Sonarr and Radarr IDs

Tracker monitors compare torrents that are already in qui. A qui torrent carries only a name, so without IDs a monitor searches Tracker B by title. Add Sonarr and Radarr in **Settings > Sonarr & Radarr** so monitors can resolve IMDb, TMDb and TVDb IDs, search by ID, and recognize listings with a different title spelling or language.

RavenSeed only reads from these instances. It never adds, changes or searches anything in Sonarr or Radarr.

## Setup

1. Choose **+ Add Sonarr** or **+ Add Radarr**.
2. Enter a name, the instance URL including any URL base, and the API key from the instance's **Settings > General**. The URL field warns when a plain `http://` address points outside your local network, since the key would travel unencrypted.
3. Choose **Test connection**. The test confirms that the URL answers as the selected type and that the key works.
4. Save. Saving clears every cached torrent identity so later checks use the new instances.

Saved API keys are never returned by the API. Leave the key blank to keep it. Changing an instance's URL or type requires entering the key again.

## How a torrent is identified

Each monitor check resolves the source torrent once, in this order. The first answer is used.

| Order | Source | Bound to | Cached for | Trust |
| --- | --- | --- | --- | --- |
| 1 | Download history for the torrent hash (all instances) | The torrent itself | 30 days | Verified |
| 2 | Release name parsed against the instance library | The name | 7 days | Inferred |
| 3 | Unique exact title lookup, with the year for movies | The name | 7 days | Inferred |

Only download history marks the source as verified. A history record exists when Sonarr or Radarr grabbed or imported that exact torrent. A cross-seeded copy has another hash, so RavenSeed also tries the torrents in the same qui instance that share its content path and exact size: they are the same files on disk. A match through such a copy is verified too, and **Media IDs** names it, for example "download history of cross-seed 882bcf29". The cross-seed index is rebuilt with each 10-minute inventory scan.

The downloaded torrent may be gone from qui, for example after you stop seeding it and keep only cross-seeds. Sonarr and Radarr's import records still hold the name and exact byte size of every file they imported. When a name match finds the movie or series in the library, RavenSeed compares those records with the torrent's files. A file with the same name and exact size verifies the torrent, and **Media IDs** names it as the imported file. A same-named file of another size never matches. A name-based identity keeps checking on later runs, so an import made afterwards still verifies it.

The qBittorrent category and the release name decide whether Sonarr or Radarr is asked first for steps 2 and 3. Categories containing `sonarr`, `tv`, `shows` or `series` mean TV; `radarr`, `movies` or `films` mean movies. Episode markers in the name, including air dates such as `2024.10.01`, always mean TV.

When nothing matches, the miss is cached for 12 hours. When an instance cannot be reached, nothing is cached and the error is shown under **Media IDs**. The check still searches by title and can find the release **Present**. It never marks a **Candidate** while the IDs are unavailable; it shows **Error** and retries after one hour.

## Cache

Identities are cached in SQLite by torrent hash and shared by every monitor that contains the torrent. Expired entries are pruned during the 10-minute inventory scan. While an inferred identity is cached, each check still reads download history, so a torrent imported later becomes verified. **Check again** re-resolves an inferred identity; a verified identity stays cached until it expires.

## Effect on monitor checks

- **Search.** Tracker B is searched by ID when its capabilities allow it. If no equivalent is found, a title search follows, because some tracker listings carry no IDs. With a verified identity, title results whose IDs contradict it are excluded. An inferred identity never excludes title results, so a wrong name match cannot hide an existing listing. The candidate's target search row, named after Tracker B (such as **Demo Beacon search**), lists every query.
- **Matching.** Listings found through matching IDs compare by ID instead of title and year. For example, `Amélie` matches `Le Fabuleux Destin d'Amélie Poulain`, and a release year one off still matches.
- **Resolution.** Download history records the release quality. When a verified torrent's name has no resolution, as SD names usually don't, that resolution is used for presence and slot checks.
- **Video bitrate.** For a verified torrent, the video bitrate Sonarr or Radarr's MediaInfo measured on the matching library file (same exact size; every video file of a pack) is cached with the identity and settles encode size tiers exactly.
- **Anime.** Sonarr's absolute episode numbers become episode mappings for that check. An absolute-numbered release such as `- 81` then compares as `S04E15`.
- **Policies.** A trump rule match is a **Trump** only when the source identity is verified. Inferred identities stay **Needs review**, and the review reason names the instance.
- **Send to UA.** The TMDb ID is prefilled from download history or a library match. Download-history IDs reach UA as they are; library-match IDs only after you tick **Confirm these IDs**. A title-lookup match is not prefilled. Clear the field to let UA identify the release itself; a different ID you enter replaces all resolved IDs.

## API

- `GET /api/v1/settings/arr`: Instances without API keys.
- `PUT /api/v1/settings/arr`: `{ "instances": [{ "id": "", "name": "Sonarr", "kind": "sonarr", "url": "http://sonarr:8989", "apiKey": "…", "enabled": true }] }`. Omit `id` for a new instance. At most 10 instances.
- `POST /api/v1/settings/arr/test`: One instance object. Tests it without saving.
- `POST /api/v1/monitor-candidates/{candidateId}/upload` sends download-history IDs automatically. `"confirmMediaIds": true` also sends IDs from a library name match; title-lookup IDs are never sent. `"clearMediaIds": true` sends no resolved IDs; a `tmdbId` in the same request still applies, and a `tmdbId` that differs from the resolved one replaces them all.

Monitor candidates include `category`, `identity` (`source`, `instance`, `instanceId`, `media`, `resolvedAt`, `expiresAt`) and `identityError`.
