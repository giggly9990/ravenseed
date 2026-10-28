// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { IndexerPolicies } from "./indexer-policies";
import { api } from "../api";
import type { IndexerPolicy } from "../types";
import { pick } from "../test/pickers";

vi.mock("../api", async (original) => ({
  ...(await original<typeof import("../api")>()),
  api: vi.fn(),
}));
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});

const slot = {
  releaseClass: true,
  resolution: true,
  source: false,
  provider: true,
  codec: true,
  hdr: true,
  cut: true,
  tvStructure: true,
  group: false,
  hybrid: false,
};
const initial: IndexerPolicy = {
  indexerId: 3,
  enabled: false,
  detectMissing: true,
  detectCoexistence: true,
  detectTrump: true,
  detectReview: true,
  slots: { WEB_DL: slot, REMUX: { ...slot } },
  encodeSizePercent: 0,
  corrections: ["REPACK"],
  webDlOverWebRip: false,
  sourcePriority: ["WEB", "BLURAY"],
  hdrRelations: [],
  groupOverNoGrp: false,
  reputableGroups: ["FLUX"],
  seasonPackOverEpisodes: false,
  reviewVisualQuality: false,
  reviewSuperiorMaster: false,
  reviewHDRComparability: false,
};
const preset = {
  id: "ruleset_1",
  name: "Ruleset 1",
  description: "Observed slot rules",
  policy: {
    ...initial,
    enabled: true,
    presetId: "ruleset_1",
    encodeSizePercent: 20,
  },
};

function setup(saved: IndexerPolicy = initial) {
  const writes: IndexerPolicy[] = [];
  vi.mocked(api).mockImplementation(async (path, options) => {
    if (path === "/indexers")
      return [
        { id: 3, name: "Target", enabled: true },
        { id: 4, name: "Other", enabled: true },
      ];
    if (path === "/indexer-policies") return [saved];
    if (path === "/indexer-policies/3" && options?.method === "PUT") {
      const body = JSON.parse(String(options.body));
      writes.push(body);
      return { ...body, snapshotId: "policy_saved" };
    }
    if (path === "/indexer-policies/3/recheck") return { queued: 4 };
    if (path === "/indexer-policies/3") return saved;
    if (path === "/indexer-policies/4") return { ...initial, indexerId: 4 };
    if (path === "/policy-presets") return [preset];
    if (path === "/monitors") return [];
    throw new Error(`Unexpected request: ${path}`);
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <IndexerPolicies ready />
    </QueryClientProvider>,
  );
  return writes;
}

it("applies a generic preset to one target and saves a policy snapshot", async () => {
  const writes = setup();
  const list = await screen.findByRole("listbox", { name: "Target indexer" });
  expect(
    (await within(list).findByRole("option", { name: /Target/ })).getAttribute(
      "aria-selected",
    ),
  ).toBe("true");
  await screen.findByRole("button", { name: "Policy preset" });
  await pick("Policy preset", "Ruleset 1");
  expect(screen.getByText("Based on Ruleset 1")).toBeTruthy();
  expect(within(list).getByText("Draft · not saved")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Save policy" }));
  await waitFor(() => expect(writes[0]?.presetId).toBe("ruleset_1"));
  expect(writes[0].indexerId).toBe(3);
  expect(writes[0].encodeSizePercent).toBe(20);
  await screen.findByText(/Policy saved/);
});

it("toggles a slot axis in the matrix and saves it in the payload", async () => {
  const writes = setup();
  const cell = (await screen.findByRole("checkbox", {
    name: "Remux: source",
  })) as HTMLInputElement;
  expect(cell.checked).toBe(false);
  expect(
    screen.getByRole("table", { name: "Slots" }).querySelectorAll("tbody tr"),
  ).toHaveLength(2);
  fireEvent.click(cell);
  expect(cell.checked).toBe(true);
  expect(
    screen.getByRole("region", { name: "Unsaved changes" }).textContent,
  ).toContain("1 unsaved change in Target policy");
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0].slots.REMUX.source).toBe(true);
  expect(writes[0].slots.WEB_DL.source).toBe(false);
});

it("edits trump rules, groups and review flags with the same payload shape", async () => {
  const writes = setup({ ...initial, snapshotId: "policy_1" });
  await screen.findByRole("checkbox", { name: "Remux: source" });
  fireEvent.click(screen.getByRole("checkbox", { name: /Needs review/ }));
  fireEvent.click(screen.getByRole("button", { name: "PROPER" }));
  fireEvent.click(screen.getByRole("button", { name: "REPACK" }));
  fireEvent.click(screen.getByLabelText("WEB-DL beats same-service WEBRip"));
  fireEvent.click(screen.getByLabelText("Reputable group beats NoGrp"));
  fireEvent.click(screen.getByLabelText("Season pack beats single episodes"));
  fireEvent.change(
    screen.getByLabelText("Encode size difference for separate slots (%)"),
    { target: { value: "12.5" } },
  );
  fireEvent.click(screen.getByRole("button", { name: "Move WEB higher" }));
  fireEvent.change(screen.getByLabelText("Add source"), {
    target: { value: "hdtv" },
  });
  fireEvent.click(
    within(screen.getByLabelText("Add source").parentElement!).getByRole(
      "button",
      { name: "Add" },
    ),
  );
  fireEvent.change(screen.getByLabelText("HDR candidate"), {
    target: { value: "dv+hdr10" },
  });
  fireEvent.change(screen.getByLabelText("HDR target"), {
    target: { value: "hdr10" },
  });
  fireEvent.keyDown(screen.getByLabelText("HDR target"), { key: "Enter" });
  const groups = screen.getByLabelText("Add reputable group");
  fireEvent.change(groups, { target: { value: "ntb, don" } });
  fireEvent.keyDown(groups, { key: "Enter" });
  fireEvent.click(screen.getByRole("button", { name: "Remove FLUX" }));
  fireEvent.click(
    screen.getByLabelText("Visual quality can't be compared from the name"),
  );
  fireEvent.click(
    screen.getByLabelText("An HDR trump needs a manual quality comparison"),
  );
  fireEvent.click(screen.getByLabelText("Source may be a superior master"));
  fireEvent.click(
    screen.getByRole("switch", { name: "Enable candidate policy" }),
  );
  expect(
    (screen.getByRole("button", { name: /Recheck/ }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "Save policy" }));
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]).toMatchObject({
    enabled: true,
    detectReview: false,
    corrections: ["PROPER"],
    webDlOverWebRip: true,
    groupOverNoGrp: true,
    seasonPackOverEpisodes: true,
    encodeSizePercent: 12.5,
    sourcePriority: ["BLURAY", "WEB", "HDTV"],
    hdrRelations: [{ from: "DV+HDR10", to: "HDR10" }],
    reputableGroups: ["NTB", "DON"],
    reviewVisualQuality: true,
    reviewHDRComparability: true,
    reviewSuperiorMaster: true,
  });
  expect(writes[0]).not.toHaveProperty("deadTorrentMaxSeeders");
  await screen.findByText(/Policy saved/);
  fireEvent.click(screen.getByRole("button", { name: /Recheck/ }));
  await screen.findByText(/Queued 4 monitor candidates/);
});

it("imports policy JSON through the dialog and keeps drafts per target", async () => {
  setup();
  await screen.findByRole("checkbox", { name: "Remux: source" });
  fireEvent.click(screen.getByRole("button", { name: "Import / export JSON" }));
  const dialog = await screen.findByRole("dialog");
  expect(
    (within(dialog).getByLabelText("Current draft JSON") as HTMLTextAreaElement)
      .value,
  ).toContain('"version": 1');
  fireEvent.change(
    within(dialog).getByLabelText("Paste policy JSON to import"),
    {
      target: { value: "{}" },
    },
  );
  fireEvent.click(
    within(dialog).getByRole("button", {
      name: "Apply imported JSON to draft",
    }),
  );
  expect(within(dialog).getByRole("alert").textContent).toContain(
    "complete slot",
  );
  fireEvent.change(
    within(dialog).getByLabelText("Paste policy JSON to import"),
    {
      target: {
        value: JSON.stringify({
          ...initial,
          slots: Object.fromEntries(
            [
              "FULL_DISC",
              "REMUX",
              "ENCODE",
              "WEB_DL",
              "WEBRIP",
              "HDTV",
              "OTHER",
            ].map((c) => [c, slot]),
          ),
          encodeSizePercent: 7,
        }),
      },
    },
  );
  fireEvent.click(
    within(dialog).getByRole("button", {
      name: "Apply imported JSON to draft",
    }),
  );
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(
    (
      screen.getByLabelText(
        "Encode size difference for separate slots (%)",
      ) as HTMLInputElement
    ).value,
  ).toBe("7");
  fireEvent.click(screen.getByRole("option", { name: /Other/ }));
  await screen.findByRole("heading", { name: "Slots" });
  fireEvent.click(screen.getByRole("option", { name: /Target/ }));
  await waitFor(() =>
    expect(
      (
        screen.getByLabelText(
          "Encode size difference for separate slots (%)",
        ) as HTMLInputElement
      ).value,
    ).toBe("7"),
  );
  fireEvent.click(screen.getByRole("button", { name: "Discard" }));
  expect(
    (
      screen.getByLabelText(
        "Encode size difference for separate slots (%)",
      ) as HTMLInputElement
    ).value,
  ).toBe("0");
});

it("edits encode size scope, slot cap, bitrate tiers and the new rule flags", async () => {
  const writes = setup({ ...initial, snapshotId: "policy_1" });
  await screen.findByRole("checkbox", { name: "Remux: source" });
  fireEvent.change(
    screen.getByLabelText("Encode size difference for separate slots (%)"),
    { target: { value: "20" } },
  );
  const scope = screen.getByLabelText("Add encode size scope");
  fireEvent.change(scope, { target: { value: "1080P:avc, nonsense" } });
  fireEvent.keyDown(scope, { key: "Enter" });
  fireEvent.change(
    screen.getByLabelText("Maximum size-separated encode slots"),
    { target: { value: "2" } },
  );
  await pick("Add encode size tier", "1080p");
  fireEvent.change(screen.getByLabelText("1080p compact tier limit (Mbps)"), {
    target: { value: "12" },
  });
  fireEvent.click(
    screen.getByLabelText("Any pack of a season blocks its single episodes"),
  );
  fireEvent.click(
    screen.getByLabelText("A dual-audio release meets a single-language one"),
  );
  fireEvent.click(
    screen.getByLabelText(
      "A 2160p WEB-DL with lossless audio may take the extra slot",
    ),
  );
  fireEvent.click(screen.getByRole("button", { name: "Save policy" }));
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]).toMatchObject({
    encodeSizePercent: 20,
    encodeSizeScope: ["1080p:AVC"],
    encodeSizeSlots: 2,
    encodeBitrateTiers: [{ resolution: "1080p", maxVideoMbps: 12 }],
    episodesBlockedByPack: true,
    reviewDualAudio: true,
    reviewLosslessWeb: true,
  });
});

it("keeps internal groups and their treatment on this tracker's policy", async () => {
  const writes = setup({ ...initial, snapshotId: "policy_1" });
  await screen.findByRole("checkbox", { name: "Remux: source" });
  const groups = screen.getByLabelText("Add internal group");
  fireEvent.change(groups, { target: { value: "home, crew" } });
  fireEvent.keyDown(groups, { key: "Enter" });
  await pick(
    "Internal releases",
    "Internals have their own slot beside non-internal releases",
  );
  fireEvent.click(screen.getByRole("button", { name: "Save policy" }));
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]).toMatchObject({
    indexerId: 3,
    internalGroups: ["HOME", "CREW"],
    internalSlots: "separate",
  });
});

it("keeps this tracker's banned groups on its policy", async () => {
  const writes = setup({ ...initial, snapshotId: "policy_1" });
  await screen.findByRole("checkbox", { name: "Remux: source" });
  const banned = screen.getByLabelText("Add banned group");
  fireEvent.change(banned, { target: { value: "yify, evo" } });
  fireEvent.keyDown(banned, { key: "Enter" });
  fireEvent.click(screen.getByRole("button", { name: "Save policy" }));
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]).toMatchObject({
    indexerId: 3,
    bannedGroups: ["YIFY", "EVO"],
  });
});

it("re-imports its own export and saves only fields the server accepts", async () => {
  const writes = setup({
    ...initial,
    slots: Object.fromEntries(
      ["FULL_DISC", "REMUX", "ENCODE", "WEB_DL", "WEBRIP", "HDTV", "OTHER"].map(
        (c) => [c, { ...slot }],
      ),
    ),
  });
  await screen.findByRole("checkbox", { name: "Remux: source" });
  fireEvent.click(screen.getByRole("button", { name: "Import / export JSON" }));
  const dialog = await screen.findByRole("dialog");
  const exported = JSON.parse(
    (within(dialog).getByLabelText("Current draft JSON") as HTMLTextAreaElement)
      .value,
  );
  exported.encodeSizePercent = 9;
  exported.slots.REMUX.unknownAxis = true;
  exported.hdrRelations = [{ from: "DV", to: "SDR", note: "extra" }];
  fireEvent.change(
    within(dialog).getByLabelText("Paste policy JSON to import"),
    { target: { value: JSON.stringify(exported) } },
  );
  fireEvent.click(
    within(dialog).getByRole("button", {
      name: "Apply imported JSON to draft",
    }),
  );
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  fireEvent.click(screen.getByRole("button", { name: "Save policy" }));
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]).not.toHaveProperty("version");
  expect(writes[0].encodeSizePercent).toBe(9);
  expect(writes[0].slots.REMUX).not.toHaveProperty("unknownAxis");
  expect(writes[0].hdrRelations).toEqual([{ from: "DV", to: "SDR" }]);
});
