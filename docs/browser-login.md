# Browser login

RavenSeed supports one administrator account. When browser credentials are configured, the server protects all app pages, assets, API docs and application APIs. Only `/login`, `/health` and the compatible `/api/v1/health` alias are public. The login page is self-contained and does not load the React application.

## Configure the administrator

Set `WEB_USERNAME` and either `WEB_PASSWORD`, `WEB_PASSWORD_FILE`, `WEB_PASSWORD_HASH` or `WEB_PASSWORD_HASH_FILE`. Passwords must contain 12 to 1024 bytes. Secret files take precedence over the corresponding environment variable. Never commit credential files.

For Docker, put the password in `secrets/web_password` and copy `docker-compose.auth.example.yml` to `docker-compose.override.yml`. The override is automatically used by ordinary `docker compose` commands and is git-ignored. Explicit `-f` commands must include that override themselves. The example sets `WEB_COOKIE_SECURE: "true"`, which suits HTTPS through a reverse proxy; for the default `http://localhost:8088` publication, change it to `"false"` as described below, or the login form is refused over plain HTTP.

Use `AUTH_MODE=session` for browser login without requiring an API token. An optional `API_TOKEN` or `API_TOKEN_FILE` enables separate automation access and must contain at least 24 characters. `AUTH_MODE=token` continues to require an API token, but browsers now need their own web credentials. Token-only deployments continue serving automation while the login page tells the operator to configure browser credentials, so the WebUI is unusable until they are set. An API bearer token does not unlock app pages or assets.

A token with `AUTH_MODE` unset or empty enables token mode; `.env.example` and `docker-compose.yml` leave it empty by default, so setting `API_TOKEN` alone is enough under Compose. A token with an explicit `AUTH_MODE=none` fails at startup rather than being ignored.

Setting browser credentials also enables authentication with `AUTH_MODE=none`. With neither credentials nor token mode, local development remains unauthenticated. Remote exposure without authentication still requires an explicit opt-in.

## HTTPS and localhost

`WEB_COOKIE_SECURE=true` is the default. Put network-accessible deployments behind HTTPS. Configure `TRUSTED_PROXIES` for the proxy's immediate address or CIDR. The proxy must preserve the browser's `Host`, or send it as `X-Forwarded-Host`, which the proxy sets itself rather than passing a client's value through (see [reverse proxies](security.md#reverse-proxies)), and overwrite `X-Forwarded-Proto` with the actual external scheme. Login accepts that header only from trusted peers. It will not present a password form over untrusted plain HTTP while secure cookies are required.

For the default localhost-only Docker publication, set `WEB_COOKIE_SECURE: "false"` in `docker-compose.override.yml` (or `WEB_COOKIE_SECURE=false` in `.env` when not using the override) and use `http://localhost:8088`. This is an explicit HTTP exception. Startup rejects insecure cookies when the declared listener or host publication is non-loopback. Change the cookie setting to `true` before exposing the app remotely.

## Sessions and logout

Login sets an HttpOnly cookie with SameSite=Strict, a host-only scope and a 12-hour absolute expiry. HTTPS sessions use the `__Host-ravenseed_session` cookie; the localhost HTTP exception uses `ravenseed_session`. Session identifiers contain 32 random bytes, and only their SHA-256 digests are stored in memory. Sessions never enter localStorage or API responses.

**Sign out** revokes the server-side session, clears its cookie and cancels requests using that session, including active event streams. Expired sessions are rejected. Restarting RavenSeed revokes all sessions. Up to 128 browser sessions are retained; additional logins revoke the oldest. API tokens are independent of browser logout.

Login and cookie-authenticated mutations require an exact same-origin Origin header, including the external scheme. GET cannot log out. Login attempts are limited to five per client per minute, where a client is an IPv4 address or an IPv6 /64, with at most two simultaneous password checks. From the fifth failure in a day, each further failure locks the client out for one minute, doubling up to one hour. Failed logins across all clients are capped at 30 per minute; clients that signed in successfully in the last 30 days are exempt from that cap, so failures from elsewhere cannot lock the owner out. Limited attempts return `429` with `Retry-After`. Forwarded client addresses follow `TRUSTED_PROXIES`. Protected responses use `Cache-Control: no-store`.

Each failed login logs one JSON line such as `{"time":"…","level":"WARN","msg":"login failed","client_ip":"203.0.113.7"}`, so a tool such as fail2ban can ban repeat offenders at the host or proxy. A filter for `docker compose logs` output or a log file could use:

```ini
[Definition]
failregex = "msg":"login failed","client_ip":"<HOST>"
```

This complements the built-in limits; it does not make RavenSeed suitable for direct internet exposure. Keep it behind a reverse proxy with TLS, ideally one that also restricts who can reach it.

## Password rotation and hashes

Replace the password secret and recreate the container with `docker compose up -d --force-recreate ravenseed`. This applies the new password and invalidates every existing session. There is no public account registration or password-reset endpoint.

To store a derived hash instead of the plaintext password, generate it through stdin:

```sh
docker run --rm -i ravenseed:local hash-password < secrets/web_password > secrets/web_password_hash
```

Mount that file and set `WEB_PASSWORD_HASH_FILE` instead of `WEB_PASSWORD_FILE`. The format is `pbkdf2-sha256$iterations$base64-salt$base64-key`, using unpadded Base64, a 16-byte salt and a 32-byte key. Generated hashes use 600,000 PBKDF2-HMAC-SHA256 iterations through Go's standard library. See [OWASP password storage guidance](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html#pbkdf2) and [origin validation guidance](https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html#verifying-the-origin-with-standard-headers).
