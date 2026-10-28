import assert from "node:assert/strict";

// Run against the seeded demo's admin port (see scripts/demo-seed.mjs). A
// fresh demo needs about eight minutes to check every torrent.
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
  const text = await res.text();
  assert.ok(res.ok, `${path}: ${res.status} ${text}`);
  return JSON.parse(text);
}
assert.equal(
  (await call("/qui/instances"))[0]?.name,
  "Demo qBittorrent (mock)",
  "Use only the isolated demo",
);

const arr = await call("/settings/arr");
const radarr = arr.instances.find((x) => x.name === "Demo Radarr");
const sonarr = arr.instances.find((x) => x.name === "Demo Sonarr");
assert.ok(radarr && sonarr, "Run scripts/demo-seed.mjs first");
assert.ok(arr.instances.every((x) => x.apiKey === undefined && x.hasApiKey));
assert.deepEqual(await call("/settings/arr/test", { ...radarr, apiKey: "" }), {
  connected: true,
});
assert.deepEqual(await call("/settings/arr/test", { ...sonarr, apiKey: "" }), {
  connected: true,
});
const wrongKind = await fetch(`${base}/api/v1/settings/arr/test`, {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({
    ...radarr,
    id: "",
    kind: "sonarr",
    apiKey: "demo-radarr-key",
  }),
});
assert.equal(wrongKind.status, 502);

const hosts = await call("/tracker-hosts");
assert.equal(hosts.find((h) => h.host === "atlas.demo")?.count, 7);
assert.equal(hosts.find((h) => h.host === "orbit.demo")?.count, 1);
const policy = await call("/indexer-policies/3");
assert.equal(policy.enabled, true);
assert.deepEqual(policy.bannedGroups, ["BADGRP"]);
const monitor = (await call("/monitors")).find(
  (m) => m.sourceTrackerHost === "atlas.demo" && m.targetIndexerId === 3,
);
assert.ok(monitor);

// The monitor worker checks one torrent per minute.
let items;
const checked = () =>
  items.length === 7 &&
  items.every(
    (c) => c.lastCheckedAt && c.status !== "checking" && c.status !== "queued",
  );
for (let i = 0; i < 600; i++) {
  items = (await call(`/monitors/${monitor.id}/candidates?limit=50`)).items;
  if (checked()) break;
  await new Promise((r) => setTimeout(r, 1000));
}
assert.ok(
  checked(),
  `monitor still checking after 10 minutes: ${items
    .filter(
      (c) =>
        !c.lastCheckedAt || c.status === "checking" || c.status === "queued",
    )
    .map((c) => c.name)
    .join(", ")}`,
);
const byHash = Object.fromEntries(items.map((c) => [c.hash.slice(0, 4), c]));
const expect = (hash, status, classification, source) => {
  const c = byHash[hash];
  assert.ok(c, `candidate ${hash}`);
  assert.equal(c.status, status, `${c.name}: ${c.message}`);
  assert.equal(c.policy?.classification, classification, c.name);
  assert.equal(c.identity?.source, source, c.name);
  assert.equal(c.identityError ?? "", "", c.name);
  return c;
};
const remux = expect("cccc", "candidate", "coexistence", "arr_history");
assert.equal(remux.identity.media.tmdbId, 603);
assert.equal(remux.identity.videoMbps, 58);
assert.equal(remux.identity.runtime, 136);
assert.ok(remux.targetQuery.startsWith("IMDB tt0133093"));
const web = expect("dddd", "present", "equivalent", "arr_parse");
assert.equal(web.identity.instance, "Demo Radarr");
const dune = expect("eeee", "occupied", "not_eligible", "arr_history");
assert.equal(
  dune.identity.importedFile,
  "Dune.2021.1080p.AMZN.WEB-DL.DDP5.1.H.264-NTb.mkv",
);
assert.equal(dune.identity.viaHash, "7".repeat(40));
assert.equal(dune.upgrade, "upgrade");
assert.ok(dune.upgradeOffer.title.includes("REPACK"));
assert.ok(dune.upgradeOffer.upgrade.some((u) => u.to === "REPACK"));
const arrival = expect("ffff", "uncertain", "possible_trump", "arr_history");
assert.equal(arrival.identity.viaHash, "a".repeat(40));
assert.equal(arrival.identity.videoMbps, 12.5);
assert.equal(arrival.upgrade, "possible");
assert.equal(arrival.upgradeOffer.group, "DON");
const banned = expect("9999", "not_eligible", "not_eligible", "arr_lookup");
assert.ok(banned.message.includes("BADGRP"));
const episode = expect("8888", "occupied", "not_eligible", "arr_history");
assert.equal(episode.identity.instance, "Demo Sonarr");
assert.equal(episode.identity.media.tvdbId, 121361);
assert.equal(episode.category, "tv-sonarr");
assert.equal(episode.upgrade, "upgrade");
assert.equal(episode.upgradeOffer.relation, "season_pack");
assert.ok(
  episode.upgradeOffer.upgrade.some(
    (u) => u.type === "tv_structure" && u.to === "season",
  ),
);
const unknown = expect("6666", "candidate", "missing_slot", "none");
assert.ok(!unknown.targetQuery.includes("IMDB"));

const upgrades = await call("/upgrades");
// Rule-backed upgrades come first.
assert.deepEqual(
  upgrades.items
    .slice(0, 2)
    .map((c) => c.hash.slice(0, 4))
    .sort(),
  ["8888", "eeee"],
);
assert.equal(upgrades.items[2].hash.slice(0, 4), "ffff");
assert.equal((await call("/upgrades?level=upgrade")).total, 2);
assert.equal((await call("/upgrades?level=possible")).total, 1);
assert.equal(
  (await call(`/upgrades?q=${encodeURIComponent("x264 DON")}`)).total,
  1,
);
await call(`/monitor-candidates/${arrival.id}/upgrade/dismiss`, {
  dismissed: true,
});
try {
  assert.equal((await call("/upgrades")).total, 2);
  assert.equal(
    (await call("/upgrades?dismissed=true")).items[0].id,
    arrival.id,
  );
} finally {
  await call(`/monitor-candidates/${arrival.id}/upgrade/dismiss`, {
    dismissed: false,
  });
}
assert.equal((await call("/upgrades")).total, 3);
const added = await call(`/monitor-candidates/${dune.id}/upgrade/add`, {
  instanceId: 1,
  category: "Movies",
  paused: true,
});
assert.equal(added.added, 1);

console.log(
  JSON.stringify({
    status: "passed",
    monitor: monitor.id,
    candidates: items.length,
    verified: items.filter((c) => c.identity?.source === "arr_history").length,
    upgrades: upgrades.total,
    mockUpgradeAdded: added.added,
  }),
);
