import assert from "node:assert/strict";

// Checks the public demo port refuses every change except the mock-only
// exploring ones (and among those, an add with Upload to target) and still
// serves the app. Run after scripts/demo-seed.mjs.
const base = process.argv[2] ?? "http://localhost:8089";
const send = (path, method, body = {}) =>
  fetch(`${base}/api/v1${path}`, {
    method,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
const get = async (path) => {
  const res = await fetch(`${base}/api/v1${path}`);
  assert.equal(res.status, 200, path);
  return res.json();
};
const instance = (await get("/qui/instances"))[0];
assert.equal(
  instance?.name,
  "Demo qBittorrent (mock)",
  "Use only the isolated demo",
);

const settings = await get("/settings");
const arr = await get("/settings/arr");
const monitor = (await get("/monitors"))[0];
assert.ok(monitor, "Run scripts/demo-seed.mjs first");
const refused = [
  ["/settings", "PUT", { ...settings, quiUrl: "http://example.invalid" }],
  ["/settings/test", "POST", { quiUrl: "http://example.invalid", apiKey: "x" }],
  ["/settings/torrents", "PUT"],
  ["/settings/matching", "PUT"],
  ["/settings/search-overrides", "PUT"],
  ["/settings/metadata", "PUT"],
  ["/settings/metadata/test", "POST"],
  ["/settings/uploads", "PUT"],
  ["/settings/uploads/test", "POST"],
  ["/settings/arr", "PUT", { instances: [] }],
  [
    "/settings/arr/test",
    "POST",
    { kind: "radarr", url: "http://example.invalid", apiKey: "x" },
  ],
  ["/indexer-policies/3", "PUT"],
  ["/indexer-policies/3", "DELETE"],
  [
    "/monitors",
    "POST",
    {
      sourceIndexerId: 2,
      sourceTrackerHost: "orbit.demo",
      targetIndexerId: 3,
      intervalHours: 24,
    },
  ],
  [`/monitors/${monitor.id}`, "PUT", { enabled: false }],
  [`/monitors/${monitor.id}`, "DELETE"],
  ["/presets", "PUT", { name: "demo", indexerIds: [1] }],
  ["/presets/demo", "DELETE"],
  ["/indexer-policies/3/recheck", "POST"],
  ["/uploads/upload_demo/cancel", "POST"],
  ["/uploads/upload_demo/retry", "POST"],
  ["/monitor-candidates/cand_demo/upload", "POST"],
  // Unlisted routes and path tricks fall to the default refusal.
  ["/searches/..%2Fpresets", "PUT"],
  ["/settings%2Ftest", "POST"],
];
for (const [path, method, body] of refused) {
  const res = await send(path, method, body);
  assert.equal(res.status, 403, `${method} ${path}`);
  assert.equal(
    (await res.json()).error.code,
    "demo_read_only",
    `${method} ${path}`,
  );
}
assert.deepEqual(await get("/settings"), settings);
assert.deepEqual(await get("/settings/arr"), arr);
assert.equal((await get("/indexer-policies/3")).enabled, true);
assert.equal((await get(`/monitors`))[0].enabled, true);

// Exploring stays possible: searches, monitor findings, upgrades and pages.
const search = await send("/searches", "POST", {
  media: { type: "movie", imdbId: "tt0133093" },
  indexerIds: [1, 2, 3],
  targetIndexerId: 3,
});
assert.ok(search.ok, `search: ${search.status}`);
let job = await search.json();
for (let attempt = 0; attempt < 100 && job.status === "running"; attempt++) {
  await new Promise((resolve) => setTimeout(resolve, 100));
  job = await get(`/searches/${job.id}`);
}
assert.equal(job.request.targetIndexerId, 3);
// Adding is allowed, but not Upload to target: the seed keeps
// Upload-Assistant off, so the app refuses the upload.
const result = (await get(`/searches/${job.id}/results`)).items[0];
assert.ok(result, "search found nothing to add");
const upload = await send(
  `/searches/${job.id}/results/${result.id}/add`,
  "POST",
  { instanceId: instance.id, paused: true, uploadAfterDownload: true },
);
assert.equal(upload.status, 400, "add with upload");
// add_failed covers other errors too: the refusal must name Upload-Assistant.
const { error } = await upload.json();
assert.equal(error.code, "add_failed");
assert.match(error.message, /Upload-Assistant/, "add with upload refusal");
assert.ok((await get(`/monitors/${monitor.id}/candidates`)).total > 0);
await get("/upgrades");
for (const path of ["/", "/settings", "/monitors", "/upgrades"]) {
  assert.equal((await fetch(base + path)).status, 200, path);
}
console.log(
  JSON.stringify({
    status: "passed",
    refused: refused.length,
    settingsUnchanged: true,
  }),
);
