// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router";
import { UploadQueue } from "./upload-queue";
import { api } from "../api";
import { pick } from "../test/pickers";
import { resetRavenErrors } from "./raven-reactions";

vi.mock("../api", async (original) => ({
  ...(await original<typeof import("../api")>()),
  api: vi.fn(),
}));
afterEach(() => {
  cleanup();
  resetRavenErrors();
  vi.mocked(api).mockReset();
});

const now = new Date().toISOString();
const baseJob = {
  id: "failed",
  title: "Movie",
  tracker: "AITHER",
  status: "failed",
  message: "UA failed",
  args: "--anon",
  attempt: 0,
  progress: 1,
  media: { type: "movie", tmdbId: 603 },
  instanceId: 1,
  createdAt: now,
  updatedAt: now,
};

type Job = typeof baseJob & Record<string, unknown>;

/** Answers GET /uploads like the server: status filter, paging, counts. */
function uploads(path: string, jobs: Job[]) {
  const url = new URL(path, "http://x");
  const statuses = url.searchParams.get("status")?.split(",");
  const limit = Number(url.searchParams.get("limit"));
  const offset = Number(url.searchParams.get("offset"));
  const matching = jobs.filter((j) => !statuses || statuses.includes(j.status));
  const statusCounts: Record<string, number> = {};
  for (const j of jobs)
    statusCounts[j.status] = (statusCounts[j.status] ?? 0) + 1;
  return {
    items: matching.slice(offset, offset + limit),
    total: matching.length,
    limit,
    offset,
    statusCounts,
  };
}

function renderQueue() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const router = createRouter({
    routeTree: createRootRoute({ component: UploadQueue }),
    history: createMemoryHistory({ initialEntries: ["/uploads"] }),
  });
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  return client;
}

it("reuploads an existing download with edited preset flags and a TMDb ID", async () => {
  const job = { ...baseJob };
  vi.mocked(api).mockImplementation(async (path, options) => {
    if (path.startsWith("/uploads?"))
      return uploads(path, [
        job,
        { ...job, id: "active", status: "uploading", title: "Active movie" },
      ]);
    if (path === "/settings/uploads")
      return {
        flagPresets: [{ id: "mod", name: "Moderation", args: "--anon --modq" }],
      };
    if (path === "/uploads/failed/retry") {
      expect(options?.method).toBe("POST");
      expect(JSON.parse(String(options?.body))).toEqual({
        attempt: 0,
        args: "--anon --modq --debug",
        tmdbId: 604,
      });
      job.status = "downloading";
      return job;
    }
    throw new Error(`Unexpected request: ${path}`);
  });
  const client = renderQueue();
  fireEvent.click(await screen.findByRole("tab", { name: /Needs you/ }));
  fireEvent.click(await screen.findByRole("button", { name: "Reupload" }));
  const dialog = within(screen.getByRole("dialog"));
  expect(
    (dialog.getByLabelText("TMDb ID", { exact: false }) as HTMLInputElement)
      .value,
  ).toBe("603");
  await waitFor(() =>
    expect(
      vi.mocked(api).mock.calls.some(([path]) => path === "/settings/uploads"),
    ).toBe(true),
  );
  await pick(
    dialog.getByRole("button", { name: "UA flag preset" }),
    "Moderation",
  );
  expect((dialog.getByLabelText("UA flags") as HTMLInputElement).value).toBe(
    "--anon --modq",
  );
  fireEvent.change(dialog.getByLabelText("UA flags"), {
    target: { value: "--anon --modq --debug" },
  });
  fireEvent.change(dialog.getByLabelText("TMDb ID", { exact: false }), {
    target: { value: "604" },
  });
  fireEvent.click(dialog.getByRole("button", { name: "Reupload" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(
    vi.mocked(api).mock.calls.filter(([path]) => path.endsWith("/retry")),
  ).toHaveLength(1);
  client.clear();
});

it("fetches each tab from the server and counts each group", async () => {
  vi.mocked(api).mockImplementation(async (path) => {
    if (path.startsWith("/uploads?"))
      return uploads(path, [
        { ...baseJob, id: "a", status: "uploading", title: "Uploading.One" },
        {
          ...baseJob,
          id: "b",
          status: "downloading",
          title: "Downloading.Two",
          progress: 0.42,
        },
        { ...baseJob, id: "c", status: "adding", title: "Adding.Three" },
        { ...baseJob, id: "d", status: "failed", title: "Failed.Four" },
        {
          ...baseJob,
          id: "e",
          status: "needs_review",
          title: "Review.Five",
        },
        { ...baseJob, id: "f", status: "finished", title: "Finished.Six" },
        {
          ...baseJob,
          id: "g",
          status: "cancelled",
          title: "Cancelled.Seven",
        },
      ]);
    throw new Error(`Unexpected request: ${path}`);
  });
  const client = renderQueue();
  const active = await screen.findByRole("tab", { name: "Active 3" });
  expect(active.getAttribute("aria-selected")).toBe("true");
  expect(screen.getByRole("tab", { name: "Needs you 2" })).toBeTruthy();
  expect(screen.getByRole("tab", { name: "Done 2" })).toBeTruthy();
  expect(
    screen.getByText("1 uploading · 1 downloading · 1 waiting · 2 need you"),
  ).toBeTruthy();

  const table = screen.getByRole("table");
  expect(within(table).getByText("Uploading.One")).toBeTruthy();
  expect(within(table).getByText("Adding.Three")).toBeTruthy();
  expect(within(table).queryByText("Failed.Four")).toBeNull();
  expect(within(table).queryByText("Finished.Six")).toBeNull();
  const progress = within(table).getByRole("progressbar", {
    name: "Download progress",
  });
  expect(progress.getAttribute("aria-valuenow")).toBe("42");
  // Only waiting downloads can be cancelled.
  expect(
    within(table).getAllByRole("button", { name: "Cancel upload" }),
  ).toHaveLength(1);

  expect(
    vi
      .mocked(api)
      .mock.calls.some(([path]) =>
        path.endsWith("&status=adding,downloading,uploading"),
      ),
  ).toBe(true);

  const hint = /Finished without a tracker result/;
  expect(screen.queryByText(hint)).toBeNull();
  fireEvent.click(screen.getByRole("tab", { name: "Done 2" }));
  expect(await within(table).findByText("Finished.Six")).toBeTruthy();
  // Finished.Six has no tracker result, as from an older Upload-Assistant.
  expect(screen.getByText(hint)).toBeTruthy();
  expect(within(table).getByText("Cancelled.Seven")).toBeTruthy();
  expect(within(table).queryByText("Uploading.One")).toBeNull();
  expect(
    within(table).getAllByRole("button", { name: "Reupload" }),
  ).toHaveLength(2);
  client.clear();
});

it("shows failed jobs with an inline xD error, retry action and details", async () => {
  vi.mocked(api).mockImplementation(async (path, options) => {
    if (path.startsWith("/uploads?"))
      return uploads(path, [
        {
          ...baseJob,
          message: "Tracker rejected upload (dupe)",
          attempt: 1,
          hash: "abc123",
          contentPath: "/downloads/movies/Movie",
          previousAttempts: [
            {
              attempt: 0,
              args: "--screens 6",
              media: { type: "movie", tmdbId: 0 },
              status: "failed",
              message: "Could not identify the title",
              trackerResult: {
                status: "banned",
                link: "https://aither.example/torrents/8",
              },
              updatedAt: now,
            },
          ],
        },
      ]);
    if (path === "/uploads/failed/cancel") {
      expect(options?.method).toBe("POST");
      return {};
    }
    if (path === "/settings/uploads") return { flagPresets: [] };
    throw new Error(`Unexpected request: ${path}`);
  });
  const client = renderQueue();
  // Nothing active in the queue, so it opens on what needs you.
  const tab = await screen.findByRole("tab", { name: "Needs you 1" });
  await waitFor(() => expect(tab.getAttribute("aria-selected")).toBe("true"));
  const alert = await screen.findByRole("alert");
  expect(
    within(alert).getByText("Tracker rejected upload (dupe)"),
  ).toBeTruthy();
  expect(
    within(alert).getByText(
      "Attempt 2 · previous attempt failed: Could not identify the title",
    ),
  ).toBeTruthy();
  expect(within(alert).getByText("xD")).toBeTruthy();

  const details = screen.getByRole("button", { name: "Details for Movie" });
  expect(details.getAttribute("aria-expanded")).toBe("false");
  fireEvent.click(details);
  expect(details.getAttribute("aria-expanded")).toBe("true");
  expect(screen.getByText("failed_retry_1")).toBeTruthy();
  expect(screen.getByText("movie/603")).toBeTruthy();
  expect(screen.getByText("/downloads/movies/Movie")).toBeTruthy();
  expect(screen.getByText("--screens 6")).toBeTruthy();
  expect(
    screen.getByRole("link", { name: "tracker: Banned" }).getAttribute("href"),
  ).toBe("https://aither.example/torrents/8");

  fireEvent.click(within(alert).getByRole("button", { name: "Reupload" }));
  const dialog = within(screen.getByRole("dialog"));
  expect(dialog.getByText("Reupload to target")).toBeTruthy();
  client.clear();
});

it("explains tracker refusals and links finished uploads", async () => {
  vi.mocked(api).mockImplementation(async (path) =>
    uploads(path, [
      {
        ...baseJob,
        id: "uploaded",
        status: "finished",
        message: "Uploaded to AITHER.",
        trackerResult: {
          status: "success",
          link: "https://aither.example/torrents/9",
          torrentId: "9",
        },
      },
      {
        ...baseJob,
        id: "dupe",
        title: "Other movie",
        status: "failed",
        message: "Not uploaded to AITHER (possible dupe): Possible dupe",
        trackerResult: { status: "dupe", message: "Possible dupe" },
      },
      {
        ...baseJob,
        id: "broken",
        title: "Broken movie",
        message: "UA exited with code 1",
      },
      {
        ...baseJob,
        id: "unsafe",
        title: "Unsafe link",
        status: "finished",
        message: "Uploaded to AITHER.",
        trackerResult: { status: "success", link: "javascript:alert(1)" },
      },
    ]),
  );
  const client = renderQueue();
  expect(await screen.findByRole("tab", { name: "Needs you 2" })).toBeTruthy();
  expect(await screen.findByText("Dupe")).toBeTruthy();
  expect(screen.getByText("Not uploaded: possible dupe")).toBeTruthy();
  // A real failure keeps its wording.
  expect(screen.getByText("Upload did not finish")).toBeTruthy();
  expect(
    screen.getByText("Not uploaded to AITHER (possible dupe): Possible dupe"),
  ).toBeTruthy();
  fireEvent.click(screen.getByRole("tab", { name: "Done 2" }));
  const tracker = await screen.findByRole("link", { name: "AITHER" });
  // An unsafe link is not rendered at all.
  expect(screen.getAllByRole("link")).toHaveLength(1);
  // UA reported the tracker outcome, so the queue does not send you to its WebUI.
  expect(screen.queryByText(/Finished without a tracker result/)).toBeNull();
  expect(tracker.getAttribute("href")).toBe(
    "https://aither.example/torrents/9",
  );
  expect(tracker.getAttribute("rel")).toContain("noreferrer");
  expect(screen.getAllByText("Uploaded")).toHaveLength(2);
  expect(screen.queryByText("Finished")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Details for Movie" }));
  expect(screen.getByText("Torrent ID")).toBeTruthy();
  client.clear();
});

it("points an empty queue at Upload-Assistant settings", async () => {
  vi.mocked(api).mockImplementation(async (path) => uploads(path, []));
  const client = renderQueue();
  const link = await screen.findByRole("link", {
    name: "Settings → Upload-Assistant",
  });
  expect(link.getAttribute("href")).toBe("/settings?section=uploads");
  expect(screen.queryByRole("tablist")).toBeNull();
  client.clear();
});

it("pages a tab with the server total, beyond the first page", async () => {
  const jobs = Array.from({ length: 25 }, (_, i) => ({
    ...baseJob,
    id: `f${i}`,
    title: `Failed.${i}`,
  }));
  jobs.push({ ...baseJob, id: "done", status: "finished", title: "Done.One" });
  vi.mocked(api).mockImplementation(async (path) => {
    if (path.startsWith("/uploads?")) return uploads(path, jobs);
    throw new Error(`Unexpected request: ${path}`);
  });
  const client = renderQueue();
  expect(await screen.findByRole("tab", { name: "Needs you 25" })).toBeTruthy();
  expect(screen.getByRole("tab", { name: "Done 1" })).toBeTruthy();
  expect(await screen.findByText("1–20 of 25")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Next" }));
  expect(await screen.findByText("21–25 of 25")).toBeTruthy();
  expect(screen.getByText("Failed.24")).toBeTruthy();
  expect(screen.queryByText("Failed.0")).toBeNull();
  expect(
    vi
      .mocked(api)
      .mock.calls.some(([path]) =>
        path.includes("offset=20&status=failed,needs_review"),
      ),
  ).toBe(true);
  // Another tab starts on its own first page.
  fireEvent.click(screen.getByRole("tab", { name: "Done 1" }));
  expect(await screen.findByText("Done.One")).toBeTruthy();
  client.clear();
});
