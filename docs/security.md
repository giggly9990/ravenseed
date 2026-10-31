# Deployment and data protection

RavenSeed is a single-administrator service. Anyone with API access can change service endpoints, read search history and submit torrents or uploads. A bearer token grants all of these capabilities.

## Network boundary

The binary defaults to a loopback listener. The Docker image listens on all container interfaces; the supplied Compose file publishes only on the host's loopback address. Compose sets `PUBLISHED_BIND_ADDRESS` to the same value as `BIND_ADDRESS`. This declaration must match the real host publication. It does not enforce a firewall or protect against other containers on the same network.

Unauthenticated non-loopback exposure refuses startup. For remote access, configure [browser login](browser-login.md), keep secure cookies enabled and terminate TLS at a trusted reverse proxy. API clients can use a separate `API_TOKEN` or `API_TOKEN_FILE`. An operator can explicitly bypass the guard with `ALLOW_UNAUTHENTICATED_REMOTE=true`, for example behind a proxy that authenticates every request. Prevent direct access around that proxy.

Without authentication, RavenSeed serves only requests addressed to a loopback name such as `localhost` or `127.0.0.1`, plus the names in `ALLOWED_HOSTS`. This blocks DNS rebinding, where a web page on another site points its own host name at your server to read or change it through your browser. Others get `403 host_rejected`; `/health` and `/api/v1/health` stay open. Set `ALLOWED_HOSTS` to a comma-separated list of host names or IPs, such as `ravenseed.lan,192.168.1.10`, when you browse by LAN name or IP after changing `BIND_ADDRESS`, or by a reverse proxy's host name. Port, letter case, a trailing dot and IPv6 brackets are ignored; URLs, paths and wildcards fail at startup. The list is unused once authentication is enabled.

When authentication is disabled and the process listens on a non-loopback address, startup logs a warning. The Docker image always listens on all container interfaces, so Compose deployments without authentication show it even with a loopback publication: other containers on shared networks can still reach the port.

For a custom Docker deployment published only on localhost, set `PUBLISHED_BIND_ADDRESS=127.0.0.1` and publish with `-p 127.0.0.1:8088:8080`. Do not make that declaration with an all-interface host publication.

qui and Upload-Assistant URLs intentionally support private hosts and Docker service names. Their settings let the administrator initiate server-side HTTP requests. RavenSeed is not suitable for untrusted users sharing one API token. Use network egress rules to restrict destinations when the deployment needs that boundary. Redirects are rejected; upstream URLs and response bodies are omitted from connection errors.

## Reverse proxies

Rate limiting allows 240 requests per minute per client address in one process. Forwarded headers are ignored by default. Set `TRUSTED_PROXIES` to a comma-separated list of proxy IP addresses or CIDRs to use `X-Forwarded-For` from those peers. The proxy must overwrite or safely append the actual client address. From a trusted proxy, the last `X-Forwarded-Host` value, when present, replaces `Host` for the `ALLOWED_HOSTS` and same-origin checks. The proxy must therefore set `X-Forwarded-Host` itself to the host the browser addressed, or remove it, and never pass a client's value through: nginx, for example, forwards client headers unchanged unless told otherwise, so add `proxy_set_header X-Forwarded-Host $http_host;`. A passed-through value lets any client name an allowed host such as `localhost`. RavenSeed walks the chain from the right and stops at the first untrusted address. Invalid chains fall back to the socket peer.

Trust only the proxy's addresses, never an arbitrary client network or `0.0.0.0/0`. The limiter and worker limits are process-local. Run one RavenSeed process per SQLite database; shared rate limits and multiple replicas are not supported.

Public `/api/v1/health` only checks the local process and SQLite. It performs no upstream requests and stays available independently of API quotas. `/api/v1/ready` uses the configured authentication and rate limiter before checking qui. A configured dependency outage returns `503`; initial setup returns `200` with `qui: "not_configured"`. Docker probes local health so a qui outage does not make its container unhealthy.

## Stored data and retention

Treat `/data/discovery.db`, its WAL and all backups as secrets. RavenSeed creates the database directory with mode `0700` and restricts the database, WAL and shared-memory files to `0600`. They can contain service credentials, tracker passkeys in download URLs, raw responses and search history. Use restricted filesystem access and encrypted storage and backups. UI URL masking only changes presentation.

Retention is opt-in. `HISTORY_RETENTION=720h` removes completed or failed searches older than 30 days and their associated analysis and events. Searches referenced by adding, downloading, uploading or needs-review jobs are retained. Finished, failed and cancelled upload jobs are deleted too once the limit has passed since they ended (their last update), so a job that failed only recently keeps its error. A finished job's submission reservation remains so pruning cannot permit a duplicate submission. A failed or cancelled job's reservation is released with it, so its torrent can be queued again, which still takes a user action and a fresh target check.

`RAW_RESULT_RETENTION=168h` removes raw tracker JSON from results and nested family/variant payloads after seven days. Normalized analysis and original download URLs remain available for Add to qui. Raw retention does not redact passkeys from those URLs. Independently of both settings, cached TMDb metadata is deleted 90 days after it expires. Deleting or rewriting a URL's authentication parameters would break the torrent handoff.

Both settings default to `0`, which disables cleanup. They accept nonnegative Go durations such as `168h`, not `7d`. Cleanup runs on startup and hourly, in batches of up to 100 searches per retention policy, with a 30-second timeout. Running searches are never pruned.

Retention performs logical deletion. It does not securely erase SQLite free pages, WAL files, snapshots or earlier backups, and does not promise immediate file-size reduction. Storage encryption and backup lifecycle controls remain necessary.
