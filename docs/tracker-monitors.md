# Tracker monitors

Tracker monitors compare torrents already in qui against a selected target indexer and collect potential Upload-Assistant candidates. No torrent is uploaded automatically.

## Setup

1. In **Tracker monitors**, choose the qui instances to scan. Only active, connected instances are offered. Change them later with **Edit instances** in the monitor's options. Monitors created before instance selection scan all instances, including ones added later.
2. Choose Tracker A's qui indexer. This records the source tracker name for the comparison.
3. Choose Tracker A's announce host from the hosts found on torrents in the chosen instances. qui does not reliably map an indexer entry to an announce host, so this explicit choice determines which existing torrents belong to Tracker A. Only the hostname is stored; announce passkeys and full URLs are not shown.
4. Choose Tracker B's enabled qui indexer and a recheck interval. The default is 24 hours, with a supported range of 6 to 168 hours.
5. Create the monitor. It scans the chosen instances immediately and rescans the inventory every 10 minutes to find new torrents. Use **Scan qui now** after adding a torrent if needed.

Saved candidates and due times survive restarts. The worker checks one due torrent per minute. Each check bypasses qui's search cache and queries only Tracker B through qui. A failed or partial search yields **Error**, with a retry after one hour. It never becomes an upload candidate from an incomplete search.

**Edit instances** either ticks **All instances, including ones added later**, which scans every active, connected instance now and in future, or lists specific instances. Instances already on the monitor stay ticked while offline, marked **offline**; untick one to remove it. Only an active, connected instance can be added. Saving with no change keeps the scope as it is. An active monitor scans added instances at once, and re-adding an instance rechecks its candidates. Torrents on removed instances are queued, turn **Gone** at their check and leave the monitor at the one after; an upload job already queued for one continues. Saving the instances clears an earlier offline error. If every chosen instance is offline, scans fail with "none of this monitor's qui instances is active and connected" until one returns.

Use **Pause rechecks** on a saved monitor to stop new inventory scans and target searches without deleting its candidates or due times. **Resume rechecks** restores the schedule; overdue candidates become eligible on the next queue tick. A search already in progress when paused may finish. Manual **Scan qui now** and **Check again** are unavailable while paused. Existing upload jobs continue independently of the monitor.

The worker parses the source torrent's release name and searches Tracker B. With [Sonarr and Radarr configured](arr-integration.md), it first resolves the torrent's IMDb, TMDb and TVDb IDs, preferring the download history for its hash, and searches by ID. A title search follows when the ID search finds no equivalent. While a configured instance is unreachable, a monitor does not mark new candidates. Movies use title and year, then title alone when nothing in scope was found; TV monitors search the series title and filter seasons and episodes locally so tracker naming differences do not hide results. For a single-episode torrent, season packs that contain its season stay in scope, so pack rules such as **Any pack of a season blocks its single episodes** and the season-pack upgrade apply. The worker then compares release metadata and exact reported byte sizes. A matching release and size is **Present**. A plausible same-size result with incomplete metadata is **Uncertain**. If no equivalent result is found after a successful search, it is a **Candidate**. These are search findings, not proof that Tracker B accepts an upload. Review tracker rules and UA's duplicate check before submitting.

When Tracker B has an enabled [Indexer Policy](indexer-policies.md), the same check also records a slot classification, evidence and the immutable policy version. The monitor retains its queue states. Policy decisions never turn a failed or partial target search into a candidate. An explicit policy recheck queues fresh searches; older findings keep their original snapshot until checked again.

The target indexer must support a title query for this workflow. Torrents whose tracker URL is missing, whose release name cannot be parsed, or whose size is unknown cannot become candidates. A removed source torrent is marked **Gone**. Source selection uses the torrent's primary tracker URL; torrents with several announce URLs may need manual review.

## Sending a candidate to Upload-Assistant

Configure UA and the Tracker B code and source instance path mapping under Settings. Choose **Send to UA** on a candidate checked in the last 48 hours. RavenSeed uses the existing torrent by hash; it does not add another copy to qui. You can provide a TMDb ID and a saved UA flag preset. The TMDb ID is prefilled when Sonarr or Radarr identified the torrent by download history or library match. IDs from download history are sent to UA as they are; IDs from a library name match are sent only after you tick **Confirm these IDs … are this release**. The upload queue verifies that the torrent and all its files are complete, maps its path, reserves the hash and target, and calls UA. An uncertain submission needs manual review and is never retried automatically. See [upload workflow](upload-workflow.md).

Deleting a monitor removes its comparison records. It does not remove torrents or existing upload jobs.

## API

- `GET /api/v1/tracker-hosts`: Announce hostname, torrent count, and `instanceCounts` (torrent count per instance ID) for active qui instances.
- `GET /api/v1/monitors`: Saved tracker pairs.
- `POST /api/v1/monitors`: `{ "sourceIndexerId": 1, "sourceTrackerHost": "announce.example", "targetIndexerId": 2, "intervalHours": 24, "instanceIds": [7] }`. `instanceIds` is required and lists active, connected qui instances; the host must appear on one of them.
- `PUT /api/v1/monitors/{monitorId}`: `{ "enabled": false }` pauses rechecks; `{ "enabled": true }` resumes them. `{ "instanceIds": [7, 8] }` replaces the scanned instances, alone or with `enabled`; instances the monitor already covers are kept even while offline, and added ones must be active and connected. `{ "instanceIds": [] }` scans all instances, including ones added later; such monitors return no `instanceIds`. Send at least one field.
- `POST /api/v1/monitors/{monitorId}/scan`: Refresh source inventory. Returns `409` while the monitor is paused and `502` when none of its instances is active and connected.
- `GET /api/v1/monitors/{monitorId}/candidates?limit=50&offset=0`: Comparison queue and findings. `q` searches release names and groups across all pages, ignoring case, separators and accents; `sort=checked_desc` or `checked_asc` orders by the last check, with unchecked torrents last; `classification=queued` lists torrents waiting for or in a check; `classification`, `releaseClass`, `resolution`, `source`, `hdr`, `group` and `policy` narrow the list.
- `GET /api/v1/monitor-candidate-counts`: `{ "monitorId": { "all": 7, "missing_slot": 1, "queued": 0, ... } }`, each monitor's total for every candidates-list filter: `all`, `missing_slot`, `trump`, `possible_trump`, `coexistence`, `equivalent`, `not_eligible`, `queued`, `errors` and `gone`.
- `POST /api/v1/monitor-candidates/{candidateId}/check`: Queue a fresh target check. Returns `409` while the monitor is paused or the torrent is already being checked.
- `POST /api/v1/monitor-candidates/{candidateId}/upload`: `{ "instanceId": 7, "tmdbId": 603, "uploadPresetId": "" }`. The TMDb ID and preset are optional; the instance must be the source torrent's instance. Resolved IDs from download history are sent automatically; add `"confirmMediaIds": true` to send IDs from a library name match. IDs from a title lookup are never sent. See [Sonarr and Radarr IDs](arr-integration.md#api) for `clearMediaIds`.
- `DELETE /api/v1/monitors/{monitorId}`: Remove the monitor and its candidate records.
- Sonarr and Radarr settings: see [Sonarr and Radarr IDs](arr-integration.md#api).

All endpoints use the normal RavenSeed authentication and origin checks.

## Presence and review findings

Title punctuation such as `FBI International` and `FBI: International`, apostrophes, accents and `&` versus `and` are normalized for comparison. Listings verified by the same media ID match regardless of title or release year. A target listing with an optional `Dual-Audio` label can still count as **Present** when its exact reported bytes and all other release metadata agree. With exact reported bytes, a resolution, codec, bit depth, audio format, web service, language (such as NORDiC), HDR format or cut named on only one side is accepted: SD names often omit the resolution, and tracker titles often drop tags the release name carries. Values named on both sides must agree. A REPACK, PROPER or similar label named on one side only, or another revision of the same label such as REPACK and REPACK3, is also accepted, because a corrected release has different bytes; different labels such as REPACK and PROPER are not. Different codecs, channels, languages, groups, web services, HDR formats, corrections, or episode sets are not ignored. Matching does not verify file contents.

Some trackers report sizes rounded for display, such as 10.63 GiB, instead of exact bytes. A listing whose size equals this release's size rounded to two decimals in GiB, GB, MiB or MB, and whose release fields agree, is **Needs review**, never **Present** and never a **Candidate**: rounding can hide a modified file.

With Indexer Policies enabled, an observed equivalent takes precedence over missing policy fields or optional review settings. An occupied slot with no observed improvement and no distinct group or source to compare is **Occupied**, not an upload candidate. A different release may still require manual comparison. In the candidate's details, the row named after the target indexer, such as **Demo Beacon search**, lists each submitted query with the returned and in-scope result counts. **Size** shows the exact source bytes. Target release details also show exact bytes.

Saved findings keep their previous analysis until checked again. Resume a paused monitor before using **Check again**, or queue rechecks from Indexer Policies. Search history must be refreshed to use the updated parser.