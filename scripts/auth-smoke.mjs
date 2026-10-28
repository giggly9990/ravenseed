// Browser-login smoke test for a local instance running AUTH_MODE=session
// (for example with docker-compose.auth.example.yml). It checks protected
// pages, assets and APIs, login, origin (CSRF) checks and logout.
//
// Usage: node scripts/auth-smoke.mjs [url] [password-file] [username]
//   url            local base URL (default http://localhost:8088); only
//                  localhost, 127.0.0.1 and [::1] are accepted
//   password-file  file holding the web password (default secrets/web_password)
//   username       web username (default admin)
// It reads no environment variables. Over plain HTTP the server needs
// WEB_COOKIE_SECURE=false: with secure cookies it refuses to sign in without
// HTTPS. The script sends the session cookie itself, so its Secure flag never
// matters.
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

const base = new URL(process.argv[2] ?? "http://localhost:8088");
assert.ok(
  ["localhost", "127.0.0.1", "[::1]"].includes(base.hostname),
  "Use only a local instance",
);
const password = (
  await readFile(process.argv[3] ?? "secrets/web_password", "utf8")
).trim();
const username = process.argv[4] ?? "admin";
const call = (path, options = {}) =>
  fetch(new URL(path, base), { redirect: "manual", ...options });
assert.equal((await call("/health")).status, 200);
assert.equal((await call("/")).status, 303);
assert.equal((await call("/api/docs")).status, 401);
assert.equal((await call("/api/openapi.json")).status, 401);
assert.equal((await call("/api/v1/settings")).status, 401);
const loginResponse = await call("/login");
assert.equal(loginResponse.headers.get("referrer-policy"), "same-origin");
const loginPage = await loginResponse.text();
assert.ok(loginPage.includes('name="password"'));
assert.ok(!loginPage.includes("/assets/"));
assert.equal(
  (
    await call("/login", {
      method: "POST",
      headers: { Origin: "null" },
      body: new URLSearchParams({ username, password }),
    })
  ).status,
  403,
);
const login = await call("/login", {
  method: "POST",
  headers: { Origin: base.origin },
  body: new URLSearchParams({ username, password, next: "/history" }),
});
assert.equal(login.status, 303);
assert.equal(login.headers.get("location"), "/history");
const setCookie = login.headers.get("set-cookie");
assert.ok(setCookie, "login did not set a session cookie");
assert.ok(
  setCookie.includes("HttpOnly") && setCookie.includes("SameSite=Strict"),
);
const cookie = setCookie.split(";")[0];
const authenticated = { headers: { Cookie: cookie } };
const app = await call("/", authenticated);
assert.equal(app.status, 200);
assert.equal(app.headers.get("referrer-policy"), "same-origin");
const html = await app.text();
const asset = html.match(/src="([^\"]+\.js)"/)?.[1];
assert.ok(asset, "app page has no script asset");
assert.equal((await call(asset)).status, 303);
assert.equal((await call(asset, authenticated)).status, 200);
assert.equal((await call("/api/docs", authenticated)).status, 200);
assert.equal((await call("/api/v1/settings", authenticated)).status, 200);
assert.equal(
  (
    await call("/logout", {
      method: "POST",
      headers: { Cookie: cookie, Origin: "https://invalid.example" },
    })
  ).status,
  403,
);
assert.equal(
  (
    await call("/logout", {
      method: "POST",
      headers: { Cookie: cookie, Origin: base.origin },
    })
  ).status,
  303,
);
assert.equal((await call("/api/v1/settings", authenticated)).status, 401);
console.log(
  JSON.stringify({
    status: "passed",
    login: true,
    protectedAssets: true,
    protectedDocs: true,
    csrf: true,
    revokedSession: true,
  }),
);
