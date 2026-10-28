# Upgrades

Trackers often replace a release when a better one is uploaded: a REPACK of the same group, a better HDR format, or another group's encode. A torrent you seed from Tracker A can then be outdated on Tracker B. **Upgrades** lists those torrents.

Upgrades come from [tracker monitor](tracker-monitors.md) checks; no extra scanning or tracker searches run. A monitor reports upgrades only when Tracker B has an enabled [indexer policy](indexer-policies.md), since the policy defines which releases share a slot. Torrents already present on Tracker B are never listed.

## Levels

- **Upgrade:** a configured policy rule ranks Tracker B's release above yours, such as a REPACK or higher revision of the same group, an HDR relation, WEB-DL over WEBRip, source priority, or a season pack over your single episodes. The reason is shown, for example `correction: original → REPACK`.
- **Possible upgrade:** a different group's release holds your slot on Tracker B and no rule settles which is better, or a rule ranks it higher but the same review checks as a trump apply: an HDR ranking (such as DV HDR10+ over DV HDR10) when **An HDR trump needs a manual quality comparison** is on, or an HDR format missing from either name. The note says what to compare before replacing.

A different group's release is not offered as a possible upgrade when its name and size already rank it below yours: an encode more than 5% smaller (fewer bits in the same slot), a WEB-DL more than 10% smaller (a lower-bitrate stream from the service; smaller gaps are extra audio or subtitle tracks), or lower-class main audio (lossless to lossy, or fewer channels). A remux's size reflects its tracks rather than picture quality, so size is not used for remuxes. Rule-backed upgrades are never filtered this way.

Releases from a group on Tracker B's banned list are never offered.

## Actions

- **Open** shows the release on Tracker B.
- **Add to qui** downloads Tracker B's release through qui. Download links carry tracker passkeys and are not stored, so Tracker B is searched again, using qui's cache, for the listing with the same title and exact size. Your current torrent is left untouched.
- **Dismiss** hides an upgrade. It returns by itself when Tracker B's release changes. **Show dismissed** lists hidden upgrades, with **Restore**.

Existing monitor findings report upgrades from their next check. Use **Recheck candidates** on the target's indexer policy to refresh them sooner.

## API

- `GET /api/v1/upgrades?level=upgrade|possible&monitorId=&q=&dismissed=true&limit=50&offset=0`: Upgrade findings across monitors, rule-backed first. `q` is at most 200 characters.
- `POST /api/v1/monitor-candidates/{candidateId}/upgrade/dismiss`: `{ "dismissed": true }` hides the current offer; `false` restores it. Returns `409 upgrade_changed` when a check replaces the offer meanwhile; refresh and retry.
- `POST /api/v1/monitor-candidates/{candidateId}/upgrade/add`: The qui add options (`instanceId`, `category`, `savePath`, `tags`, `paused`). The instance must be connected and active, or the request returns `400`.
