import assert from "node:assert/strict";

const base = process.argv[2] ?? "http://localhost:8090";
async function call(path, body, method = "POST") {
  const res = await fetch(
    `${base}/api/v1${path}`,
    body === undefined
      ? undefined
      : {
          method,
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(body),
        },
  );
  assert.ok(res.ok, `${path}: ${res.status}`);
  return res.json();
}
const terminal = ["finished", "failed", "needs_review", "cancelled"];
// Polls the upload queue until the job settles. Upload-Assistant submissions
// are spaced UA_SUBMISSION_INTERVAL (1m) apart, so allow well over a minute.
async function settled(id, seconds = 150) {
  let job;
  for (let i = 0; i < seconds; i++) {
    job = (await call("/uploads?limit=200")).items.find((j) => j.id === id);
    assert.ok(job, `upload job ${id} is missing from the queue`);
    if (terminal.includes(job.status)) return job;
    await new Promise((r) => setTimeout(r, 1000));
  }
  assert.fail(
    `upload job ${id} still ${job.status} after ${seconds}s: ${job.message}`,
  );
}
assert.equal(
  (await call("/qui/instances"))[0]?.name,
  "Demo qBittorrent (mock)",
  "Use only the isolated demo",
);
// Restore the demo's torrent and upload settings when the smoke test ends.
const originalPreferences = await call("/settings/torrents");
const originalUploads = await call("/settings/uploads");
const config = {
  enabled: true,
  url: "http://mock-qui:7476/ua",
  token: "demo-upload-key",
  trackers: [{ indexerId: 3, code: "DEMO" }],
  paths: [{ instanceId: 1, from: "/downloads", to: "/data" }],
  flagPresets: [
    { id: "private", name: "Private moderation", args: "--anon --modq" },
  ],
  defaultFlagPresetId: "private",
};
try {
  const preferences = {
    defaultInstanceId: 1,
    categories: [{ instanceId: 1, enabled: ["Movies"] }],
  };
  await call("/settings/torrents", preferences, "PUT");
  assert.deepEqual(await call("/settings/torrents"), preferences);
  const settings = await call("/settings/uploads", config, "PUT");
  assert.equal(settings.token, undefined);
  assert.equal(settings.hasToken, true);
  await call("/settings/uploads/test", { ...config, token: "" });
  let search = await call("/searches", {
    media: { type: "movie", imdbId: "tt0133093" },
    indexerIds: [1, 2, 3],
    targetIndexerId: 3,
    filters: { minimumCoverage: 2 },
    refresh: true,
  });
  for (let i = 0; i < 100 && search.status === "running"; i++) {
    await new Promise((r) => setTimeout(r, 100));
    search = await call(`/searches/${search.id}`);
  }
  assert.equal(search.status, "completed");
  const result = (await call(`/searches/${search.id}/results`)).items.find(
    (r) => r.indexerId === 1,
  );
  const disabled = await fetch(
    `${base}/api/v1/searches/${search.id}/results/${result.id}/add`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ category: "TV" }),
    },
  );
  assert.equal(disabled.status, 400);
  const add = await call(`/searches/${search.id}/results/${result.id}/add`, {
    category: "Movies",
    uploadAfterDownload: true,
    paused: false,
  });
  assert.ok(add.uploadJobId);
  // The mock Upload-Assistant reports this result for the DEMO tracker.
  const trackerResult = {
    status: "success",
    link: "https://demo.example/torrents/1",
    torrentId: "1",
  };
  let job = await settled(add.uploadJobId);
  assert.equal(job.status, "finished", job.message);
  assert.deepEqual(job.trackerResult, trackerResult);
  assert.equal(job.uploadPath, "/data/Demo/movie.mkv");
  assert.equal(job.tracker, "DEMO");
  assert.equal(job.hash, "b".repeat(40));
  assert.equal(job.instanceId, 1);
  assert.equal(job.flagPresetName, "Private moderation");
  assert.equal(job.args, "--anon --modq");
  assert.equal(job.media.tmdbId, 603);
  await call(`/uploads/${job.id}/retry`, {
    attempt: job.attempt ?? 0,
    args: "--modq",
    tmdbId: 603,
  });
  job = await settled(add.uploadJobId);
  assert.equal(job.status, "finished", job.message);
  assert.equal(job.attempt, 1);
  assert.equal(job.args, "--modq");
  assert.equal(job.previousAttempts.length, 1);
  assert.equal(job.previousAttempts[0].args, "--anon --modq");
  assert.deepEqual(job.previousAttempts[0].trackerResult, trackerResult);
  assert.deepEqual(job.trackerResult, trackerResult);
  const stale = await fetch(`${base}/api/v1/uploads/${job.id}/retry`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ attempt: 0, args: "--anon" }),
  });
  assert.equal(stale.status, 400);
  console.log(
    JSON.stringify({
      status: "passed",
      job: job.id,
      uploadStatus: job.status,
      pathMapping: true,
      tokenRedacted: true,
      reupload: true,
      tmdb: true,
      trackerResult: true,
    }),
  );
} finally {
  await call("/settings/torrents", originalPreferences, "PUT");
  // The API never returns the token: an empty one keeps the stored token, which
  // is only the original when the URL is unchanged. Otherwise clear it.
  const keepToken =
    originalUploads.hasToken && originalUploads.url === config.url;
  if (originalUploads.hasToken && !keepToken)
    console.warn(
      "Upload-Assistant token was replaced by the demo token; re-enter it in Settings",
    );
  await call(
    "/settings/uploads",
    {
      ...originalUploads,
      token: "",
      clearToken: !keepToken,
      enabled: originalUploads.enabled && keepToken,
    },
    "PUT",
  );
}
