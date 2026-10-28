// Configures the isolated demo for the tracker monitor workflow: mock Radarr
// and Sonarr, an indexer policy on Demo Beacon, and a Demo Atlas monitor.
// Running it again leaves existing demo settings in place, except that
// Upload-Assistant is always turned off: with it off, the public add route
// refuses Upload to target. It waits up to two minutes for RavenSeed and the
// mock qui to start. A failed check exits nonzero with its reason.
import assert from "node:assert/strict";

const base = process.argv[2] ?? "http://localhost:8090";
const mock = process.env.MOCK_QUI_URL ?? "http://mock-qui:7476";
// Requests get 30 seconds, or what is left of the start-up deadline while
// waiting for it.
let limit = 30_000;
const timeout = () => AbortSignal.timeout(limit);
async function call(path, body, method = "POST") {
  const init =
    body === undefined
      ? {}
      : {
          method,
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(body),
        };
  const response = await fetch(`${base}/api/v1${path}`, {
    ...init,
    signal: timeout(),
  });
  const text = await response.text();
  assert.ok(response.ok, `${path}: HTTP ${response.status} ${text}`);
  return JSON.parse(text);
}

// Plain Compose runs the seed once (restart: "no"), so wait for RavenSeed
// and the mock qui to start rather than failing on a slow start.
const deadline = Date.now() + 120_000;
/** Polls `check` until it returns a value, or fails with `reason` once the
 * shared two-minute start-up deadline passes; no attempt runs past it. */
async function ready(reason, check) {
  for (;;) {
    let last = "";
    limit = Math.max(1000, Math.min(30_000, deadline - Date.now()));
    try {
      const value = await check();
      if (value !== undefined) return value;
    } catch (error) {
      last = `: ${error.message}`;
    } finally {
      limit = 30_000;
    }
    assert.ok(
      Date.now() < deadline,
      `${reason} (gave up after 2 minutes${last})`,
    );
    await new Promise((resolve) => setTimeout(resolve, 2000));
  }
}

await ready("RavenSeed did not become healthy", async () => {
  const health = await (
    await fetch(`${base}/api/v1/health`, { signal: timeout() })
  ).json();
  return health.status === "ok" || undefined;
});
const instances = await ready(
  "RavenSeed did not reach a connected mock qui",
  async () => {
    const list = await call("/qui/instances");
    return list[0]?.connected ? list : undefined;
  },
);
assert.equal(
  instances[0].name,
  "Demo qBittorrent (mock)",
  "Seed only the isolated demo",
);

const arr = await call("/settings/arr");
const missing = [
  {
    name: "Demo Radarr",
    kind: "radarr",
    url: `${mock}/radarr`,
    apiKey: "demo-radarr-key",
    enabled: true,
  },
  {
    name: "Demo Sonarr",
    kind: "sonarr",
    url: `${mock}/sonarr`,
    apiKey: "demo-sonarr-key",
    enabled: true,
  },
].filter((v) => !arr.instances.some((x) => x.name === v.name));
if (missing.length)
  await call(
    "/settings/arr",
    { instances: [...arr.instances, ...missing] },
    "PUT",
  );

const target = 3;
if (!(await call(`/indexer-policies/${target}`)).enabled) {
  const preset = (await call("/policy-presets")).find(
    (p) => p.id === "ruleset_4",
  );
  assert.ok(preset?.policy, "Policy preset ruleset_4 is missing");
  await call(
    `/indexer-policies/${target}`,
    { ...preset.policy, indexerId: target, bannedGroups: ["BADGRP"] },
    "PUT",
  );
}

const uploads = await call("/settings/uploads");
if (uploads.enabled)
  await call("/settings/uploads", { ...uploads, enabled: false }, "PUT");

let monitor = (await call("/monitors")).find(
  (m) => m.sourceTrackerHost === "atlas.demo" && m.targetIndexerId === target,
);
if (!monitor) {
  const host = await ready(
    "qui lists no atlas.demo torrents; is mock-qui up to date?",
    async () =>
      (await call("/tracker-hosts")).find((h) => h.host === "atlas.demo"),
  );
  monitor = await call("/monitors", {
    sourceIndexerId: 1,
    sourceTrackerHost: "atlas.demo",
    targetIndexerId: target,
    intervalHours: 24,
    instanceIds: Object.keys(host.instanceCounts).map(Number),
  });
}
console.log(
  JSON.stringify({
    status: "seeded",
    monitor: monitor.id,
    arr: ["Demo Radarr", "Demo Sonarr"],
    policyIndexer: target,
    uploadAssistant: "disabled",
  }),
);
