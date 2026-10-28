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
import type { ReactNode } from "react";
import { UpgradesPage } from "./upgrades-page";
import { api } from "../../api";
import { pick } from "../../test/pickers";

vi.mock("../../api", async (original) => ({
  ...(await original<typeof import("../../api")>()),
  api: vi.fn(),
}));
vi.mock("@tanstack/react-router", () => ({
  Link: ({ children }: { children: ReactNode }) => <a>{children}</a>,
}));
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});

const monitor = {
  id: "mon1",
  sourceIndexerId: 1,
  sourceIndexerName: "Tracker A",
  sourceTrackerHost: "a.example",
  targetIndexerId: 2,
  targetIndexerName: "Tracker B",
  intervalHours: 24,
  enabled: true,
  quiUrl: "",
  createdAt: "",
};
const upgrade = {
  id: "c1",
  monitorId: "mon1",
  instanceId: 7,
  hash: "a".repeat(40),
  name: "Odd.Thomas.2013.1080p.BluRay.x264-GRP",
  size: 9000,
  status: "occupied",
  message: "",
  media: { type: "movie" },
  nextCheckAt: "",
  matchCount: 0,
  upgrade: "upgrade",
  upgradeOffer: {
    id: "t1",
    title: "Odd.Thomas.2013.REPACK.1080p.BluRay.x264-GRP",
    indexer: "Tracker B",
    size: 9100,
    infoUrl: "https://b.example/torrents/1",
    relation: "same_slot",
    upgrade: [{ type: "correction", from: "original", to: "REPACK" }],
  },
};

function setup() {
  const calls: { path: string; body?: unknown }[] = [];
  vi.mocked(api).mockImplementation(async (path, options) => {
    calls.push({
      path,
      body: options?.body ? JSON.parse(String(options.body)) : undefined,
    });
    if (path === "/monitors") return [monitor];
    if (path.startsWith("/upgrades"))
      return { items: [upgrade], total: 1, limit: 50, offset: 0 };
    if (path === "/qui/instances")
      return [{ id: 7, name: "Main client", connected: true, isActive: true }];
    if (path === "/qui/instances/7/categories") return ["Movies"];
    if (path === "/settings/torrents")
      return { defaultInstanceId: 0, categories: [] };
    if (path === "/settings/uploads") return { trackers: [], paths: [] };
    return { added: 1 };
  });
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <UpgradesPage />
    </QueryClientProvider>,
  );
  return calls;
}

it("lists upgrades with their reason and filters by level and monitor", async () => {
  const calls = setup();
  const row = (await screen.findByText(upgrade.name)).closest("tr")!;
  expect(within(row).getByText("Upgrade")).toBeTruthy();
  expect(within(row).getByText(/correction: original → REPACK/)).toBeTruthy();
  expect(
    within(row).getByRole("link", { name: /Open/ }).getAttribute("href"),
  ).toBe("https://b.example/torrents/1");
  fireEvent.click(screen.getByRole("button", { name: "Possible" }));
  await pick("Monitor", "Tracker A → Tracker B");
  await waitFor(() =>
    expect(
      calls.some(
        (c) =>
          c.path.includes("level=possible") &&
          c.path.includes("monitorId=mon1"),
      ),
    ).toBe(true),
  );
});

it("dismisses an upgrade and adds the target's release through its own route", async () => {
  const calls = setup();
  const row = (await screen.findByText(upgrade.name)).closest("tr")!;
  fireEvent.click(within(row).getByRole("button", { name: "Dismiss" }));
  await waitFor(() =>
    expect(
      calls.find((c) => c.path === "/monitor-candidates/c1/upgrade/dismiss")
        ?.body,
    ).toEqual({ dismissed: true }),
  );
  fireEvent.click(within(row).getByRole("button", { name: "Add to qui" }));
  expect(screen.queryByText("Upload to target after download")).toBeNull();
  await pick("Destination instance", "Main client");
  fireEvent.click(screen.getByRole("button", { name: "Add torrent" }));
  await screen.findByText("qui accepted the torrent. Check its status in qui.");
  expect(
    calls.find((c) => c.path === "/monitor-candidates/c1/upgrade/add")?.body,
  ).toMatchObject({ instanceId: 7, paused: true });
});

it("refetches and explains when the offer changed before a dismiss", async () => {
  const { ApiError } =
    await vi.importActual<typeof import("../../api")>("../../api");
  const calls = setup();
  const base = vi.mocked(api).getMockImplementation()!;
  vi.mocked(api).mockImplementation(async (path, options) => {
    if (path.endsWith("/upgrade/dismiss")) {
      calls.push({ path });
      throw new ApiError(409, "upgrade_changed", "offer changed");
    }
    return base(path, options);
  });
  const row = (await screen.findByText(upgrade.name)).closest("tr")!;
  const before = calls.filter((c) => c.path.startsWith("/upgrades")).length;
  fireEvent.click(within(row).getByRole("button", { name: "Dismiss" }));
  expect(
    await screen.findByText(/changed while you were looking/),
  ).toBeTruthy();
  await waitFor(() =>
    expect(
      calls.filter((c) => c.path.startsWith("/upgrades")).length,
    ).toBeGreaterThan(before),
  );
});

it("moves back to the last page when dismissals shrink the list", async () => {
  let total = 120;
  const calls: string[] = [];
  vi.mocked(api).mockImplementation(async (path) => {
    calls.push(path);
    if (path === "/monitors") return [monitor];
    if (path.startsWith("/upgrades")) {
      const offset = Number(
        new URLSearchParams(path.split("?")[1]).get("offset"),
      );
      return {
        items: offset < total ? [upgrade] : [],
        total,
        limit: 50,
        offset,
      };
    }
    return [];
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <UpgradesPage />
    </QueryClientProvider>,
  );
  await screen.findByText(upgrade.name);
  fireEvent.click(screen.getByRole("button", { name: "Next" }));
  fireEvent.click(await screen.findByRole("button", { name: "Next" }));
  await screen.findByText("101–120 of 120");
  total = 60;
  await client.invalidateQueries({ queryKey: ["upgrades"] });
  await screen.findByText("51–60 of 60");
  expect(calls.some((p) => p.includes("offset=50"))).toBe(true);
});
