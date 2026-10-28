// @vitest-environment jsdom
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from "@tanstack/react-router";
import { MonitorsPage } from "./monitors-page";
import { api, ApiError } from "../../api";
import { closePicker, openPicker, pick } from "../../test/pickers";

vi.mock("../../api", async (original) => ({
  ...(await original<typeof import("../../api")>()),
  api: vi.fn(),
}));
let client: QueryClient | undefined;
beforeEach(() => {
  vi.mocked(api).mockReset();
  vi.spyOn(window, "scrollTo").mockImplementation(() => {});
});
afterEach(() => {
  cleanup();
  client?.clear();
});

const monitor = {
  id: "mon1",
  sourceIndexerId: 1,
  sourceIndexerName: "A",
  sourceTrackerHost: "a.example",
  targetIndexerId: 2,
  targetIndexerName: "B",
  intervalHours: 24,
  enabled: true,
  createdAt: new Date().toISOString(),
  instanceIds: [7],
};
const candidate = {
  id: "cand1",
  monitorId: "mon1",
  instanceId: 7,
  hash: "a".repeat(40),
  name: "The.Matrix.1999.1080p.WEB-DL-NTb",
  size: 1000,
  status: "candidate",
  message: "No matching release found",
  media: { type: "movie", title: "The Matrix" },
  nextCheckAt: new Date().toISOString(),
  lastCheckedAt: new Date().toISOString(),
  matchCount: 0,
};
const uploadSettings = {
  enabled: true,
  trackers: [{ indexerId: 2, code: "AITHER" }],
  paths: [{ instanceId: 7, from: "/downloads", to: "/data" }],
  flagPresets: [],
};

const classes = [
  "all",
  "missing_slot",
  "trump",
  "possible_trump",
  "coexistence",
  "equivalent",
  "not_eligible",
  "queued",
  "errors",
  "gone",
];
/** GET /monitor-candidate-counts: every class at `n`, plus overrides. */
const countRow = (n: number, overrides: Record<string, number> = {}) => ({
  ...Object.fromEntries(classes.map((c) => [c, n])),
  ...overrides,
});

type Handler = (
  path: string,
  options?: RequestInit,
) => unknown | Promise<unknown>;

/** Common fixtures; `handle` gets first pick at each request. */
function mockApi(handle: Handler) {
  vi.mocked(api).mockImplementation(async (path, options) => {
    const handled = await handle(path, options);
    if (handled !== undefined) return handled;
    if (path === "/indexers")
      return [
        { id: 1, name: "A", enabled: true, backend: "", capabilities: [] },
        { id: 2, name: "B", enabled: true, backend: "", capabilities: [] },
      ];
    if (path === "/tracker-hosts")
      return [{ host: "a.example", count: 1, instanceCounts: { 7: 1 } }];
    if (path === "/qui/instances")
      return [{ id: 7, name: "Homelab", connected: true, isActive: true }];
    if (path === "/settings/uploads") return uploadSettings;
    if (path === "/monitor-candidate-counts") return { mon1: countRow(1) };
    if (path.startsWith("/monitors/mon1/candidates"))
      return path.includes("q=")
        ? { items: [], total: 0, limit: 50, offset: 0 }
        : { items: [candidate], total: 1, limit: 50, offset: 0 };
    throw new Error(`Unexpected request: ${path}`);
  });
}

async function renderPage() {
  const root = createRootRoute({ component: Outlet });
  const monitors = createRoute({
    getParentRoute: () => root,
    path: "/monitors",
    component: MonitorsPage,
  });
  const uploads = createRoute({
    getParentRoute: () => root,
    path: "/uploads",
    component: () => <p>Upload queue</p>,
  });
  const router = createRouter({
    routeTree: root.addChildren([monitors, uploads]),
    history: createMemoryHistory({ initialEntries: ["/monitors"] }),
  });
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  await act(async () => {
    await router.load();
  });
  return client;
}

async function openOptions(label = "Monitor options") {
  const trigger = await screen.findByRole("button", { name: label });
  fireEvent.keyDown(trigger, { key: "ArrowDown" });
  return screen.findByRole("menu");
}

const calls = (match: (path: string, options?: RequestInit) => boolean) =>
  vi.mocked(api).mock.calls.filter(([path, options]) => match(path, options));

it("creates a tracker pair in a dialog, pauses it, and queues an existing candidate for UA", async () => {
  let saved = false;
  let enabled = true;
  mockApi((path, options) => {
    if (path === "/qui/instances")
      return [
        { id: 7, name: "Homelab", connected: true, isActive: true },
        { id: 8, name: "Seedbox", connected: true, isActive: true },
        { id: 9, name: "Offline", connected: false, isActive: true },
      ];
    if (path === "/tracker-hosts")
      return [
        { host: "a.example", count: 3, instanceCounts: { 7: 1, 8: 2 } },
        { host: "b.example", count: 1, instanceCounts: { 8: 1 } },
      ];
    if (path === "/monitors" && options?.method === "POST") {
      expect(JSON.parse(String(options.body))).toEqual({
        sourceIndexerId: 1,
        sourceTrackerHost: "a.example",
        targetIndexerId: 2,
        intervalHours: 24,
        instanceIds: [7],
      });
      saved = true;
      return monitor;
    }
    if (path === "/monitors") return saved ? [{ ...monitor, enabled }] : [];
    if (path === "/monitors/mon1" && options?.method === "PUT") {
      enabled = JSON.parse(String(options.body)).enabled;
      return { ...monitor, enabled };
    }
    if (path === "/monitor-candidates/cand1/upload") {
      expect(options?.method).toBe("POST");
      expect(JSON.parse(String(options?.body))).toEqual({
        instanceId: 7,
        tmdbId: 603,
        uploadPresetId: "",
      });
      return { id: "upload1", status: "downloading" };
    }
  });
  await renderPage();
  await screen.findByRole("heading", { name: "No tracker pairs yet" });
  fireEvent.click(screen.getAllByRole("button", { name: "New monitor" })[0]);
  const create = within(await screen.findByRole("dialog"));
  const submit = create.getByRole("button", {
    name: "Create monitor",
  }) as HTMLButtonElement;
  expect(submit.disabled).toBe(true);
  // Only active, connected instances can be chosen.
  await create.findByRole("checkbox", { name: "Seedbox" });
  expect(create.queryByRole("checkbox", { name: "Offline" })).toBeNull();
  fireEvent.click(create.getByRole("checkbox", { name: "Homelab" }));
  await pick("Tracker A indexer", "A");
  // Hosts and counts follow the chosen instances.
  await openPicker("Tracker A announce host");
  expect(
    screen.queryByRole("menuitemradio", { name: /b\.example/ }),
  ).toBeNull();
  closePicker();
  await pick("Tracker A announce host", "a.example · 1 torrents");
  await pick("Tracker B target indexer", "B");
  await waitFor(() => expect(submit.disabled).toBe(false));
  // Unchoosing every instance drops the host it no longer offers.
  fireEvent.click(create.getByRole("checkbox", { name: "Homelab" }));
  await waitFor(() => expect(submit.disabled).toBe(true));
  fireEvent.click(create.getByRole("checkbox", { name: "Homelab" }));
  await pick("Tracker A announce host", "a.example · 1 torrents");
  await waitFor(() => expect(submit.disabled).toBe(false));
  fireEvent.click(submit);
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());

  // The new monitor is selected and its candidates load.
  const card = await screen.findByRole("button", { name: "A → B" });
  expect(card.getAttribute("aria-pressed")).toBe("true");
  await screen.findByText(/a\.example · Homelab · every 24 h/);
  fireEvent.click(await screen.findByRole("button", { name: candidate.name }));
  const detail = within(
    screen.getByRole("complementary", { name: "Candidate detail" }),
  );

  fireEvent.click(
    within(await openOptions()).getByRole("menuitem", {
      name: "Pause rechecks",
    }),
  );
  await screen.findByText("Paused");
  expect(
    (screen.getByRole("button", { name: "Scan qui now" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
  expect(
    (detail.getByRole("button", { name: "Check again" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
  fireEvent.click(
    within(await openOptions()).getByRole("menuitem", {
      name: "Resume rechecks",
    }),
  );
  await waitFor(() => expect(screen.queryByText("Paused")).toBeNull());
  expect(enabled).toBe(true);

  fireEvent.click(await screen.findByRole("button", { name: "Send to UA" }));
  const dialog = within(screen.getByRole("dialog"));
  fireEvent.change(
    dialog.getByLabelText("TMDb ID (optional, recommended for UA)"),
    { target: { value: "603" } },
  );
  await waitFor(() =>
    expect(
      (
        dialog.getByRole("button", {
          name: "Queue upload",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(false),
  );
  fireEvent.click(dialog.getByRole("button", { name: "Queue upload" }));
  await waitFor(() =>
    expect(calls((p) => p === "/monitor-candidates/cand1/upload")).toHaveLength(
      1,
    ),
  );
});

it("drops an instance that goes offline from the new monitor's selection", async () => {
  let seedboxUp = true;
  mockApi((path, options) => {
    if (path === "/qui/instances")
      return [
        { id: 7, name: "Homelab", connected: true, isActive: true },
        { id: 8, name: "Seedbox", connected: seedboxUp, isActive: true },
      ];
    if (path === "/tracker-hosts")
      return [{ host: "a.example", count: 3, instanceCounts: { 7: 1, 8: 2 } }];
    if (path === "/monitors" && options?.method === "POST") return monitor;
    if (path === "/monitors") return [];
  });
  const qc = await renderPage();
  await screen.findByRole("heading", { name: "No tracker pairs yet" });
  fireEvent.click(screen.getAllByRole("button", { name: "New monitor" })[0]);
  const create = within(await screen.findByRole("dialog"));
  fireEvent.click(await create.findByRole("checkbox", { name: "Homelab" }));
  fireEvent.click(create.getByRole("checkbox", { name: "Seedbox" }));
  await pick("Tracker A indexer", "A");
  await pick("Tracker A announce host", "a.example · 3 torrents");
  await pick("Tracker B target indexer", "B");
  seedboxUp = false;
  await act(async () => {
    await qc.invalidateQueries({ queryKey: ["qui-instances"] });
  });
  await waitFor(() =>
    expect(create.queryByRole("checkbox", { name: "Seedbox" })).toBeNull(),
  );
  fireEvent.click(create.getByRole("button", { name: "Create monitor" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  const [post] = calls((p, o) => p === "/monitors" && o?.method === "POST");
  expect(JSON.parse(String(post[1]?.body))).toMatchObject({
    sourceTrackerHost: "a.example",
    instanceIds: [7],
  });
});

it("lists hosts once an instance is ticked and preselects a lone instance", async () => {
  mockApi((path) => {
    if (path === "/monitors") return [];
  });
  await renderPage();
  await screen.findByRole("heading", { name: "No tracker pairs yet" });
  fireEvent.click(screen.getAllByRole("button", { name: "New monitor" })[0]);
  const create = within(await screen.findByRole("dialog"));
  const homelab = (await create.findByRole("checkbox", {
    name: "Homelab",
  })) as HTMLInputElement;
  expect(homelab.checked).toBe(true);
  fireEvent.click(homelab);
  expect(homelab.checked).toBe(false);
  await create.findByRole("button", {
    name: "Tracker A announce host",
    description: "Tick an instance to list its tracker hosts",
  });
  fireEvent.click(homelab);
  await create.findByRole("button", {
    name: "Tracker A announce host",
    description: "Choose host found in qui",
  });
});

/** Mocks a monitor scanning `ids`, opens Edit instances and returns the
 * dialog plus the PUT bodies sent. */
async function openEditInstances(
  ids: number[] | undefined,
  options: {
    enabled?: boolean;
    reject?: string;
    online?: () => number[];
    lastError?: string;
  } = {},
) {
  const bodies: unknown[] = [];
  let lastError: string | undefined;
  const online = options.online ?? (() => [7, 8]);
  mockApi((path, init) => {
    if (path === "/qui/instances")
      return [
        { id: 7, name: "Homelab", isActive: true },
        { id: 8, name: "Seedbox", isActive: true },
        { id: 9, name: "Attic", isActive: true },
      ].map((i) => ({ ...i, connected: online().includes(i.id) }));
    if (path === "/monitors")
      return [
        {
          ...monitor,
          enabled: options.enabled ?? true,
          instanceIds: ids,
          lastError,
        },
      ];
    if (path === "/monitors/mon1" && init?.method === "PUT") {
      const body = JSON.parse(String(init.body));
      bodies.push(body);
      if (options.reject)
        throw new ApiError(400, "invalid_request", options.reject);
      ids = body.instanceIds.length ? body.instanceIds : undefined;
      lastError = options.lastError;
      return { ...monitor, instanceIds: ids, lastError };
    }
  });
  await renderPage();
  fireEvent.click(
    within(await openOptions()).getByRole("menuitem", {
      name: "Edit instances",
    }),
  );
  const edit = within(await screen.findByRole("dialog"));
  const box = (name: string) =>
    edit.getByRole("checkbox", { name }) as HTMLInputElement;
  const saveButton = () =>
    edit.getByRole("button", { name: "Save instances" }) as HTMLButtonElement;
  return { edit, box, saveButton, bodies };
}

it("keeps an all-instances monitor unless a fixed list is saved", async () => {
  const { edit, box, saveButton, bodies } = await openEditInstances(undefined);
  await screen.findByText(/a\.example · all instances · every 24 h/);
  const all = box("All instances, including ones added later");
  expect(all.checked).toBe(true);
  expect(edit.queryByRole("checkbox", { name: "Homelab" })).toBeNull();
  // Saving without a change would turn "all" into a fixed list.
  expect(saveButton().disabled).toBe(true);
  fireEvent.click(all);
  // Every available instance starts ticked: that is what it scans now.
  await waitFor(() => expect(box("Homelab").checked).toBe(true));
  expect(box("Seedbox").checked).toBe(true);
  expect(edit.queryByRole("checkbox", { name: /Attic/ })).toBeNull();
  fireEvent.click(box("Homelab"));
  fireEvent.click(saveButton());
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(bodies).toEqual([{ instanceIds: [8] }]);
  await screen.findByText(/a\.example · Seedbox · every 24 h/);
});

it("switches a scoped monitor to all instances", async () => {
  const { box, saveButton, bodies } = await openEditInstances([7]);
  await waitFor(() => expect(box("Homelab").checked).toBe(true));
  expect(saveButton().disabled).toBe(true);
  fireEvent.click(box("All instances, including ones added later"));
  fireEvent.click(saveButton());
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(bodies).toEqual([{ instanceIds: [] }]);
  await screen.findByText(/a\.example · all instances · every 24 h/);
});

it("keeps instances already on a monitor while they are offline", async () => {
  const { edit, box, saveButton, bodies } = await openEditInstances([7, 9]);
  await edit.findByText("Added instances are scanned at once.", {
    exact: false,
  });
  await waitFor(() => expect(box("Attic offline").checked).toBe(true));
  expect(box("Homelab").checked).toBe(true);
  expect(box("Seedbox").checked).toBe(false);
  fireEvent.click(box("Seedbox"));
  fireEvent.click(saveButton());
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(bodies).toEqual([{ instanceIds: [7, 9, 8] }]);
});

it("drops a kept offline instance and shows the offline error the save returns", async () => {
  const offline =
    "none of this monitor's qui instances is active and connected";
  const { box, saveButton, bodies } = await openEditInstances([7, 9], {
    online: () => [8],
    lastError: offline,
  });
  await waitFor(() => expect(box("Homelab offline").checked).toBe(true));
  expect(box("Attic offline").checked).toBe(true);
  // Unticking a kept offline instance removes it; it cannot come back unless
  // it is online again.
  fireEvent.click(box("Attic offline"));
  expect(box("Attic offline").checked).toBe(false);
  fireEvent.click(saveButton());
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(bodies).toEqual([{ instanceIds: [7] }]);
  await screen.findByText(offline);
});

it("explains an empty selection of a paused monitor's instances", async () => {
  const { edit, box, saveButton } = await openEditInstances([7], {
    enabled: false,
  });
  await waitFor(() => expect(box("Homelab").checked).toBe(true));
  expect(edit.queryByText(/scanned at once/)).toBeNull();
  edit.getByText(/Torrents on removed instances leave the monitor/);
  fireEvent.click(box("Homelab"));
  edit.getByText("Tick at least one instance, or choose all instances.");
  expect(saveButton().disabled).toBe(true);
});

it("shows a rejected edit and refreshes the instance list", async () => {
  let online = [7, 8];
  const { edit, box, saveButton, bodies } = await openEditInstances([7], {
    reject: "qui instance 8 is not active and connected",
    online: () => online,
  });
  await waitFor(() => expect(box("Homelab").checked).toBe(true));
  fireEvent.click(box("Seedbox"));
  online = [7];
  const fetched = calls((p) => p === "/qui/instances").length;
  fireEvent.click(saveButton());
  await edit.findByText("qui instance 8 is not active and connected");
  expect(bodies).toEqual([{ instanceIds: [7, 8] }]);
  await waitFor(() =>
    expect(calls((p) => p === "/qui/instances").length).toBeGreaterThan(
      fetched,
    ),
  );
  // Seedbox was never on the monitor, so it cannot be added offline.
  await waitFor(() =>
    expect(edit.queryByRole("checkbox", { name: /Seedbox/ })).toBeNull(),
  );
  expect(screen.getByRole("dialog")).toBeTruthy();
});

it("cancels an instance edit without saving", async () => {
  const { edit, box, bodies } = await openEditInstances([7]);
  await waitFor(() => expect(box("Homelab").checked).toBe(true));
  fireEvent.click(box("Seedbox"));
  fireEvent.click(edit.getByRole("button", { name: "Cancel" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(bodies).toEqual([]);
  expect(calls((_, o) => o?.method === "PUT")).toHaveLength(0);
});

it("selects the first monitor, filters by classification chip and keeps the extra filters", async () => {
  mockApi((path) => {
    if (path === "/monitors") return [monitor];
  });
  await renderPage();
  await screen.findByRole("button", { name: candidate.name });
  fireEvent.click(screen.getByRole("button", { name: /^Missing slot/ }));
  await waitFor(() =>
    expect(
      calls(
        (p) =>
          p.startsWith("/monitors/mon1/candidates?limit=50&") &&
          p.includes("classification=missing_slot"),
      ).length,
    ).toBeGreaterThan(0),
  );
  expect(
    screen
      .getByRole("button", { name: /^Missing slot/ })
      .getAttribute("aria-pressed"),
  ).toBe("true");

  fireEvent.click(screen.getByRole("button", { name: "More filters" }));
  await pick("Resolution filter", "1080p");
  fireEvent.change(screen.getByLabelText("Group"), {
    target: { value: "NTb" },
  });
  await waitFor(() =>
    expect(
      calls(
        (p) =>
          p.includes("classification=missing_slot") &&
          p.includes("resolution=1080p") &&
          p.includes("group=NTb"),
      ).length,
    ).toBeGreaterThan(0),
  );

  // Name search runs on the server across every page.
  fireEvent.change(screen.getByLabelText("Filter by name or group"), {
    target: { value: "scrubs s06" },
  });
  await waitFor(() =>
    expect(
      calls(
        (p) =>
          p.includes("offset=0") &&
          p.includes("q=scrubs+s06") &&
          p.includes("group=NTb"),
      ).length,
    ).toBeGreaterThan(0),
  );
  await screen.findByText("Nothing matches these filters.");
  expect(screen.queryByRole("button", { name: candidate.name })).toBeNull();
});

it("runs the detail panel actions and confirms before deleting", async () => {
  let deleted = false;
  mockApi((path, options) => {
    if (path === "/monitors") return deleted ? [] : [monitor];
    if (path === "/monitor-candidates/cand1/check") {
      expect(options).toMatchObject({ method: "POST", body: "{}" });
      return { queued: true };
    }
    if (path === "/monitors/mon1/scan") return { scanned: true };
    if (path === "/monitors/mon1" && options?.method === "DELETE") {
      deleted = true;
      return {};
    }
  });
  await renderPage();
  await screen.findByText("Select a release to see why it was flagged.");
  fireEvent.click(await screen.findByRole("button", { name: candidate.name }));
  const detail = within(
    screen.getByRole("complementary", { name: "Candidate detail" }),
  );
  detail.getByText("Homelab");
  detail.getByText("1,000 B", { exact: false });

  fireEvent.click(detail.getByRole("button", { name: "Check again" }));
  await waitFor(() =>
    expect(calls((p) => p === "/monitor-candidates/cand1/check")).toHaveLength(
      1,
    ),
  );

  fireEvent.click(
    detail.getByRole("button", { name: "Send to Upload-Assistant" }),
  );
  const dialog = within(await screen.findByRole("dialog"));
  await dialog.findByText("Upload-Assistant is enabled");
  fireEvent.click(dialog.getByRole("button", { name: "Cancel" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());

  fireEvent.click(detail.getByRole("button", { name: "Close detail" }));
  screen.getByText("Select a release to see why it was flagged.");

  fireEvent.click(screen.getByRole("button", { name: "Scan qui now" }));
  await waitFor(() =>
    expect(
      calls((p, o) => p === "/monitors/mon1/scan" && o?.method === "POST"),
    ).toHaveLength(1),
  );

  fireEvent.click(
    within(await openOptions()).getByRole("menuitem", { name: "Delete" }),
  );
  const confirm = within(await screen.findByRole("dialog"));
  expect(deleted).toBe(false);
  fireEvent.click(confirm.getByRole("button", { name: "Delete monitor" }));
  await screen.findByRole("heading", { name: "No tracker pairs yet" });
  expect(deleted).toBe(true);
});

it("shows got coke? when the uploadable count grows, never on first load", async () => {
  let missing = 2;
  mockApi((path) => {
    if (path === "/monitors") return [monitor];
    if (path === "/monitor-candidate-counts")
      return { mon1: countRow(1, { missing_slot: missing }) };
  });
  const qc = await renderPage();
  await screen.findByRole("button", { name: /^Missing slot\s*2/ });
  expect(screen.queryByText("got coke?")).toBeNull();
  missing = 5;
  await act(async () => {
    await qc.invalidateQueries({ queryKey: ["candidate-count"] });
  });
  await screen.findByText("got coke?");
  // Once in the bubble, once in the live region.
  expect(screen.getAllByText(/3 opportunities on B/)).toHaveLength(2);
  fireEvent.click(screen.getByRole("button", { name: /^All/ }));
  fireEvent.click(screen.getByRole("button", { name: "show them" }));
  expect(
    screen
      .getByRole("button", { name: /^Missing slot/ })
      .getAttribute("aria-pressed"),
  ).toBe("true");
});

it("filters queued torrents and sorts by the last check", async () => {
  mockApi((path) => {
    if (path === "/monitors") return [monitor];
  });
  await renderPage();
  await screen.findByRole("button", { name: candidate.name });
  fireEvent.click(screen.getByRole("button", { name: /^Queued/ }));
  await waitFor(() =>
    expect(
      calls((p) => p.includes("classification=queued")).length,
    ).toBeGreaterThan(0),
  );
  const header = screen.getByRole("button", { name: /^Checked/ });
  fireEvent.click(header);
  await waitFor(() =>
    expect(
      calls(
        (p) =>
          p.includes("classification=queued") &&
          p.includes("sort=checked_desc"),
      ).length,
    ).toBeGreaterThan(0),
  );
  expect(header.closest("th")?.getAttribute("aria-sort")).toBe("descending");
  fireEvent.click(header);
  await waitFor(() =>
    expect(calls((p) => p.includes("sort=checked_asc")).length).toBeGreaterThan(
      0,
    ),
  );
  fireEvent.click(header);
  expect(header.closest("th")?.getAttribute("aria-sort")).toBe("none");
});

it("counts every class of every monitor in one request", async () => {
  const second = {
    ...monitor,
    id: "mon2",
    sourceIndexerName: "C",
    targetIndexerName: "D",
  };
  mockApi((path) => {
    if (path === "/monitors") return [monitor, second];
    if (path === "/monitor-candidate-counts")
      return { mon1: countRow(1), mon2: countRow(4, { all: 40 }) };
  });
  await renderPage();
  await screen.findByRole("button", { name: candidate.name });
  // The second card shows its totals from the same response.
  await screen.findByText("40");
  expect(screen.getByText("12")).toBeTruthy();
  expect(calls((p) => p === "/monitor-candidate-counts")).toHaveLength(1);
  expect(calls((p) => p.includes("candidates?limit=1&"))).toHaveLength(0);
});

it("counts again when a monitor is missing from the counts", async () => {
  let fetched = 0;
  mockApi((path) => {
    if (path === "/monitors") return [monitor];
    // The first count predates the monitor.
    if (path === "/monitor-candidate-counts")
      return fetched++ ? { mon1: countRow(3) } : {};
  });
  await renderPage();
  await screen.findByRole("button", { name: /^All\s*3/ });
  expect(calls((p) => p === "/monitor-candidate-counts")).toHaveLength(2);
});
