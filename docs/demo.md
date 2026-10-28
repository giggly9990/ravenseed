# Demo

The demo runs RavenSeed against mock services, with no real trackers, qBittorrent, Sonarr, Radarr, TMDb or Upload-Assistant. It is safe to show publicly: visitors can search and explore, but cannot change settings.

```sh
docker compose -p ravenseed-demo -f docker-compose.yml -f docker-compose.demo.yml up -d --build
```

Requires Docker Compose 2.24.4 or newer for `!override`.

## What runs

| Service | Purpose |
| --- | --- |
| `ravenseed` | The app (image `ravenseed-demo/app`), with its own `demo-data` volume. |
| `mock-qui` | Mock qui with three indexers (Demo Atlas, Demo Orbit, Demo Beacon), one qBittorrent instance, TMDb, Radarr, Sonarr and Upload-Assistant. |
| `demo-proxy` | The page visitors open, on port 8089. Only an allowlist of changes passes through it ([demo/nginx.conf](../demo/nginx.conf)); see Ports. It forwards each visitor's address so rate limits apply per visitor. `CF-Connecting-IP` is believed only from Cloudflare's published ranges and from the edge network (`10.247.90.0/24`, except its gateway); any other client is identified by its own address. It has the fixed address `DEMO_PROXY_IP` (default `10.247.89.250`), the only proxy the app trusts to forward addresses. |
| `demo-seed` | Runs once at startup: adds the mock Radarr and Sonarr, enables a policy on Demo Beacon (Ruleset 4, with `BADGRP` banned), creates a Demo Atlas → Demo Beacon monitor and turns Upload-Assistant off. It keeps other settings already in place, then exits with code 0; Portainer shows it as exited, which is expected. It waits up to two minutes for RavenSeed and mock-qui to start. If a check fails, such as RavenSeed never becoming healthy or mock-qui listing no `atlas.demo` torrents within that time, it logs the reason and exits nonzero; the Portainer stack retries it at most five times. |

Two networks with fixed subnets, so Docker never picks one that clashes with a real network:

- `10.247.89.0/24` holds every service. Set `DEMO_SUBNET` if the range is in use, together with a `DEMO_PROXY_IP` inside the new range.
- `ravenseed-demo-edge`, `10.247.90.0/24`, holds only `demo-proxy`. A tunnel container joins this one. Set `DEMO_EDGE_SUBNET` if the range is in use, and change the edge subnet and gateway entries in the `geo $cf_trusted` block of `demo/nginx.conf` to match, or tunnel visitors share one rate limit.

## Ports

- **8089, public.** Open [http://localhost:8089](http://localhost:8089). Reads (`GET`, `HEAD`) are open. The only writes allowed are login and logout, starting a search, adding a search result to qui (without **Upload to target**: Upload-Assistant stays off, so that add is refused with `add_failed`), resolving media, a policy dry-run (`evaluate`), a monitor scan, a candidate **Check again**, and upgrade dismiss and add; they reach only the mocks. Every other request returns `403` with the code `demo_read_only`: settings, connection tests (which would otherwise send requests to any URL a visitor enters), indexer policy saves and rechecks, presets, monitor changes, sending candidates to Upload-Assistant, and upload cancel and retry. Searches visitors start, and ended upload jobs, are deleted after 24 hours (`HISTORY_RETENTION=24h`). To publish beyond this machine, set `DEMO_BIND_ADDRESS`, for example `DEMO_BIND_ADDRESS=0.0.0.0`, and add the host name or IP visitors use to `DEMO_ALLOWED_HOSTS`, keeping `ravenseed`, for example `DEMO_ALLOWED_HOSTS=ravenseed,demo.example.org`. The app runs without authentication, so it answers other host names with `403 host_rejected`.
- **8090, admin.** Bound to `127.0.0.1` with full access, for the seed and smoke scripts. It runs with `AUTH_MODE=none`: anything that can reach it can change every setting, so keep the binding on `127.0.0.1` and never publish it or route a tunnel to it.

## Portainer behind a Cloudflare tunnel

[demo/portainer-stack.yml](../demo/portainer-stack.yml) runs the same images with no host ports and no admin port. The app, mocks and seed sit on an internal network with no internet access. Only `demo-proxy` is on `ravenseed-demo-edge`, so a tunnel joined to it reaches the read-only proxy and nothing else.

1. **Build the images** on any amd64 machine with Docker:

   ```sh
   docker compose -p ravenseed-demo -f docker-compose.yml -f docker-compose.demo.yml build
   mkdir -p dist
   docker save -o dist/ravenseed-demo-images.tar ravenseed-demo/app:latest ravenseed-demo/mock-qui:latest ravenseed-demo/proxy:latest ravenseed-demo/seed:latest
   ```

2. **Import** `dist/ravenseed-demo-images.tar` in Portainer under **Images > Import**. All four images arrive in one upload.
3. **Create the stack** under **Stacks > Add stack > Web editor**, pasting `demo/portainer-stack.yml`. Deploy it. The stack creates the `ravenseed-demo-edge` network, and `demo-seed` configures the demo within a minute.
4. **Connect cloudflared.** In the stack that runs your tunnel container, add the network and attach the container to it:

   ```yaml
   services:
     cloudflared:
       networks:
         - ravenseed-demo-edge   # plus the networks it already uses
   networks:
     ravenseed-demo-edge:
       external: true
   ```

   Redeploy that stack after the demo stack exists. A one-off `docker network connect ravenseed-demo-edge <cloudflared container>` also works, but it's lost when the container is recreated.
5. **Route the hostname.** In Cloudflare Zero Trust, under the tunnel's **Public Hostname**, point the hostname at `http://demo-proxy:8080`. Add the hostname to the stack variable `DEMO_ALLOWED_HOSTS`, keeping `ravenseed`, for example `ravenseed,demo.example.org`, and redeploy; otherwise visitors get `403 host_rejected`.

To update, import a new tar, then open the stack and choose **Update the stack** with **Re-pull image** off: the images exist only on this host, so a pull would fail. The `demo-data` volume keeps monitor findings, so a restart needs no new eight-minute check run. To reset the demo, remove the stack with its volume.

## What to show

- **Search.** Search `tt0133093`: three families appear, with two target opportunities and an extra byte-size variant.
- **Tracker monitors.** The worker checks one torrent per minute, so a fresh demo takes about eight minutes to show every result. The volume keeps them across restarts. Each torrent seeded from `atlas.demo` shows one outcome:

| Torrent | Media IDs | Result on Demo Beacon |
| --- | --- | --- |
| The Matrix 2160p REMUX | Radarr download history | Candidate: Beacon only has another slot |
| The Matrix 1080p WEB-DL NTb | Radarr name match | Present |
| Dune 1080p AMZN WEB-DL NTb | Imported file name and size (grabbed torrent gone) | Occupied; **Upgrade** to Beacon's REPACK |
| Arrival 1080p BluRay SPARKS | Download history of its `orbit.demo` cross-seed; 12.5 Mbps from MediaInfo | Needs review; **Possible upgrade** to DON |
| Arrival 720p BluRay BADGRP | Radarr title lookup | Not eligible: banned group |
| Game of Thrones S01E01 1080p | Sonarr download history | Occupied; **Upgrade** to Beacon's season 1 pack |
| Night of the Living Dead | None | Candidate, from a title search |

- **Upgrades.** Lists Dune and Game of Thrones (rule-backed) and Arrival (possible).
- **Settings > Sonarr & Radarr.** Shows the two mock instances with their keys hidden.

## Tests

Run against the admin port while the demo is up:

```sh
node scripts/smoke.mjs            # search workflow, matching settings, presets, torrent handoff
node scripts/upload-smoke.mjs     # Upload-Assistant handoff and retry
node scripts/monitor-smoke.mjs    # monitor results, media IDs, upgrades, arr connection tests
node scripts/demo-readonly-smoke.mjs   # public port refuses changes (uses 8089)
```

`monitor-smoke.mjs` waits until every torrent is checked. `upload-smoke.mjs` changes torrent defaults and Upload-Assistant settings, so it runs only through the admin port, and restores them when it ends. One exception: a saved Upload-Assistant token for a different URL than the mock's cannot be restored, because the API never returns it, so the script clears it, leaves the upload queue disabled and prints a warning to re-enter it. It passes once per demo volume: its fixture torrent keeps an upload reservation, so a second run on the same volume ends in `needs_review` by design. Reset the demo (`down -v`) before running it again.

## Stop or reset

```sh
docker compose -p ravenseed-demo -f docker-compose.yml -f docker-compose.demo.yml down      # keep data
docker compose -p ravenseed-demo -f docker-compose.yml -f docker-compose.demo.yml down -v   # delete demo data
```
