import assert from "node:assert/strict";

const base = process.argv[2] ?? "http://localhost:8090";
async function request(path, body) {
  const response = await fetch(
    `${base}/api/v1${path}`,
    body === undefined
      ? undefined
      : {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(body),
        },
  );
  assert.ok(response.ok, `${path}: HTTP ${response.status}`);
  return response.json();
}
async function finished(job) {
  for (let attempt = 0; attempt < 100 && job.status === "running"; attempt++) {
    await new Promise((resolve) => setTimeout(resolve, 100));
    job = await request(`/searches/${job.id}`);
  }
  assert.equal(job.status, "completed");
  assert.equal(job.indexerErrors.length, 0);
  return job;
}

assert.deepEqual(await request("/health"), { status: "ok", database: "ok" });
assert.equal((await request("/ready")).qui, "connected");
const settings = await request("/settings");
assert.equal(settings.hasApiKey, true);
assert.equal(settings.apiKey, undefined);
assert.equal((await request("/indexers")).length, 3);
const resolvedMovie = await request("/media/resolve", {
  id: "imdb:tt0133093",
  refresh: true,
});
assert.equal(resolvedMovie.title, "The Matrix");
assert.equal(resolvedMovie.tmdbId, 603);
assert.equal(
  (await request("/media/resolve", { id: "tmdb:603", type: "movie" })).cached,
  true,
);
const resolvedSeason = await request("/media/resolve", {
  id: "1399",
  type: "tv",
  season: 1,
});
assert.equal(resolvedSeason.tvdbId, 121361);
assert.equal(resolvedSeason.seasonDetails.episodeCount, 10);
assert.equal(settings.metadata.hasToken, true);
assert.equal(settings.metadata.token, undefined);

const input = {
  media: { type: "movie", imdbId: "tt0133093" },
  indexerIds: [1, 2, 3],
  targetIndexerId: 3,
  filters: { minimumCoverage: 2 },
};
const job = await finished(
  await request("/searches", { ...input, refresh: true }),
);
assert.equal(job.resultCount, 8);
assert.equal(job.familyCount, 3);
assert.equal(job.candidateCount, 2);
assert.equal(job.media.title, "The Matrix");
assert.ok(job.indexers.every((i) => i.strategy.method === "imdb"));
const families = await request(`/searches/${job.id}/families`);
assert.equal(families.total, 3);
for (const sort of [
  "coverage",
  "quality",
  "seeders",
  "size",
  "newest",
  "target",
]) {
  const descending = await request(
    `/searches/${job.id}/families?sort=${sort}&order=desc`,
  );
  const ascending = await request(
    `/searches/${job.id}/families?sort=${sort}&order=asc`,
  );
  assert.deepEqual(
    ascending.items.map((f) => f.id),
    descending.items.map((f) => f.id).reverse(),
  );
  const first = await request(
    `/searches/${job.id}/families?sort=${sort}&order=asc&limit=1`,
  );
  assert.equal(first.items[0].id, ascending.items[0].id);
}
assert.ok(families.items.every((f) => f.variants.every((v) => v.targetStatus)));
assert.equal(
  (await request(`/searches/${job.id}/families?codec=HEVC`)).total,
  1,
);
assert.equal(families.items[0].coverage, 3);
assert.equal(families.items.filter((f) => f.variants.length === 2).length, 1);
const remux = families.items.find((f) => f.canonical.releaseType === "REMUX");
assert.equal(remux.variants[0].size, 82123456789);
assert.equal(remux.variants[1].size, 82123456889);
assert.ok(remux.results.some((r) => r.confidence === "STRONG"));
assert.equal((await request(`/searches/${job.id}/candidates`)).total, 2);
assert.equal(
  (await request(`/searches/${job.id}/families?limit=1&offset=1`)).items.length,
  1,
);
assert.equal(
  (await request(`/searches/${job.id}/families?resolution=720p`)).total,
  0,
);
assert.equal(
  (await request(`/searches/${job.id}/families/${remux.id}`)).id,
  remux.id,
);
const raw = await request(`/searches/${job.id}/results`);
assert.equal(raw.total, 8);
assert.ok(raw.items.every((r) => r.raw && r.parserOutput && r.parsed));
const events = await (
  await fetch(`${base}/api/v1/searches/${job.id}/events`)
).text();
assert.ok(events.includes("event: search.completed"));
assert.ok(events.includes("event: search.snapshot"));
const cached = await request("/searches", input);
assert.equal(cached.cacheHit, true);
assert.equal(cached.id, job.id);
const tv = await finished(
  await request("/searches", {
    ...input,
    media: { type: "tv", tmdbId: 1399, season: 1 },
    refresh: true,
  }),
);
assert.equal(tv.candidateCount, 2);
const episode = await finished(
  await request("/searches", {
    ...input,
    media: { type: "tv", tmdbId: 1399, season: 1, episode: 2 },
    refresh: true,
  }),
);
assert.equal(episode.candidateCount, 2);
const noTarget = await finished(
  await request("/searches", { ...input, targetIndexerId: 0, refresh: true }),
);
assert.equal(noTarget.request.targetIndexerId, 0);
assert.equal(noTarget.candidateCount, 0);
for (const path of [
  "/",
  "/settings",
  "/history",
  "/api/docs",
  "/api/openapi.json",
  "/docs-assets/swagger-ui-bundle.js",
]) {
  const response = await fetch(base + path);
  assert.equal(response.status, 200, path);
}
const homepage = await (await fetch(base + "/")).text();
assert.ok(homepage.includes("<title>RavenSeed</title>"));
const spec = await (await fetch(base + "/api/openapi.json")).json();
assert.equal(spec.info.title, "RavenSeed API");
assert.ok(spec.paths["/api/v1/settings/matching"]);
async function saveRules(rules) {
  const response = await fetch(base + "/api/v1/settings/matching", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(rules),
  });
  assert.equal(response.status, 200);
  return response.json();
}
try {
  const rules = {
    ...settings.matching,
    matchAlternateTitles: true,
    sizeTolerancePercent: 0.2,
    groupAliases: { OLDGROUP: "GROUP" },
    ignoredTags: ["CUSTOMTAG"],
    episodeMappings: [{ tmdbId: 99999, absolute: 13, season: 2, episode: 1 }],
  };
  const updated = await saveRules(rules);
  assert.equal(updated.matching.groupAliases.OLDGROUP, "GROUP");
  assert.equal(updated.apiKey, undefined);
  const freshRules = await finished(await request("/searches", input));
  assert.notEqual(freshRules.id, job.id);
  assert.equal(freshRules.matchingRules.sizeTolerancePercent, 0.2);
  assert.equal(
    (await request(`/searches/${job.id}`)).matchingRules.sizeTolerancePercent,
    settings.matching.sizeTolerancePercent,
  );
  const alternate = await finished(
    await request("/searches", {
      ...input,
      media: { type: "movie", imdbId: "tt9999999" },
      refresh: true,
    }),
  );
  assert.equal(alternate.familyCount, 1);
  assert.equal(alternate.candidateCount, 1);
  const advancedFamilies = await request(`/searches/${alternate.id}/families`);
  assert.equal(advancedFamilies.items[0].confidence, "EXACT");
  assert.ok(
    advancedFamilies.items[0].evidence.includes(
      "alternate_titles_same_verified_id",
    ),
  );
  assert.ok(
    advancedFamilies.items[0].results.some((r) =>
      r.normalizationEvidence.includes("explicit_group_alias"),
    ),
  );
  const anime = await finished(
    await request("/searches", {
      ...input,
      media: { type: "tv", tmdbId: 99999, season: 2, episode: 1 },
      refresh: true,
    }),
  );
  assert.equal(anime.familyCount, 1);
  assert.equal(anime.candidateCount, 1);
} finally {
  await saveRules(settings.matching);
}
const instances = await request("/qui/instances");
const categories = await request(
  `/qui/instances/${instances[0].id}/categories`,
);
assert.deepEqual(categories, ["Movies", "TV", "movies", "radarr", "tv-sonarr"]);
assert.equal(
  instances[0]?.name,
  "Demo qBittorrent (mock)",
  "Torrent handoff smoke test requires the isolated demo stack",
);
const results = await request(`/searches/${job.id}/results`);
assert.ok(
  results.items.some((r) => r.downloadFactor === 0 && r.uploadFactor === 2),
);
assert.ok(results.items.some((r) => r.downloadFactor === 0.5));
assert.equal(
  (await request(`/searches/${job.id}/families?resolution=2160p`)).total,
  1,
);
assert.equal(
  (await request(`/searches/${job.id}/families?resolution=1080p`)).total,
  2,
);
assert.equal((await request(`/searches/${job.id}/families`)).total, 3);
const added = await request(
  `/searches/${job.id}/results/${results.items[0].id}/add`,
  { instanceId: instances[0].id, paused: true, category: "Movies" },
);
assert.equal(added.added, 1);
const presetResponse = await fetch(base + "/api/v1/presets", {
  method: "PUT",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({
    name: "Smoke " + Date.now(),
    indexerIds: [1, 2, 3],
    targetIndexerId: 3,
  }),
});
assert.equal(presetResponse.status, 200);
const preset = await presetResponse.json();
try {
  assert.ok((await request("/presets")).some((p) => p.id === preset.id));
} finally {
  assert.equal(
    (await fetch(`${base}/api/v1/presets/${preset.id}`, { method: "DELETE" }))
      .status,
    200,
  );
}
console.log(
  JSON.stringify(
    {
      status: "passed",
      movie: job.id,
      tv: tv.id,
      rawResults: job.resultCount,
      families: job.familyCount,
      candidates: job.candidateCount,
      flags: true,
      presets: true,
      mockTorrentAdded: added.added,
    },
    null,
    2,
  ),
);
