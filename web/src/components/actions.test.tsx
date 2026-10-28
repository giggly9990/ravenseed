// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useState } from "react";
import { AddTorrent } from "./add-torrent";
import { IndexerPresets } from "./indexer-presets";
import { api } from "../api";
import { pick, openPicker, closePicker } from "../test/pickers";
import type { Result } from "../types";

vi.mock("../api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../api")>()),
  api: vi.fn(),
}));
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
  localStorage.clear();
});
function Wrapper({ children }: { children: React.ReactNode }) {
  const [client] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: { retry: false },
          mutations: { retry: false },
        },
      }),
  );
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}
const result = {
  id: "picked",
  indexer: "Atlas",
  title: "Example.1080p-GROUP",
  size: 1000,
  hasDownloadUrl: true,
  raw: { download_volume_factor: 0 },
} as Result;

it("sends only the picked result after an explicit submission", async () => {
  vi.mocked(api).mockImplementation(async (path) =>
    path === "/qui/instances"
      ? [{ id: 7, name: "Main client", connected: true, isActive: true }]
      : path === "/qui/instances/7/categories"
        ? ["Movies", "TV"]
        : { added: 1 },
  );
  render(<AddTorrent jobId="saved" result={result} />, { wrapper: Wrapper });
  expect(api).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "Add to qui" }));
  expect(
    vi.mocked(api).mock.calls.every(([, options]) => !options?.method),
  ).toBe(true);
  await pick("Destination instance", "Main client");
  await pick("Category", "Movies");
  fireEvent.click(screen.getByRole("button", { name: "Add torrent" }));
  await screen.findByText("qui accepted the torrent. Check its status in qui.");
  const posts = vi
    .mocked(api)
    .mock.calls.filter(([, options]) => options?.method === "POST");
  expect(posts).toHaveLength(1);
  expect(posts[0][0]).toBe("/searches/saved/results/picked/add");
  expect(JSON.parse(posts[0][1]!.body as string)).toEqual({
    instanceId: 7,
    category: "Movies",
    savePath: "",
    tags: "",
    paused: true,
  });
});

it("disables adding when the server has no download link", () => {
  render(
    <AddTorrent jobId="saved" result={{ ...result, hasDownloadUrl: false }} />,
    { wrapper: Wrapper },
  );
  const add = screen.getByRole("button", { name: "Add to qui" });
  expect(add.hasAttribute("disabled")).toBe(true);
  expect(add.getAttribute("title")).toBe("qui did not supply a download link");
});

it("reports an add failure without retrying automatically", async () => {
  vi.mocked(api).mockImplementation(async (path) => {
    if (path === "/settings/torrents")
      return { defaultInstanceId: 0, categories: [] };
    if (path === "/qui/instances")
      return [{ id: 7, name: "Main client", connected: true, isActive: true }];
    if (path === "/qui/instances/7/categories") return [];
    throw new Error("qui did not confirm the add; check qui before retrying");
  });
  render(<AddTorrent jobId="saved" result={result} />, { wrapper: Wrapper });
  fireEvent.click(screen.getByRole("button", { name: "Add to qui" }));
  await pick("Destination instance", "Main client");
  fireEvent.click(screen.getByRole("button", { name: "Add torrent" }));
  expect((await screen.findByRole("alert")).textContent).toContain(
    "check qui before retrying",
  );
  expect(
    vi
      .mocked(api)
      .mock.calls.filter(([, options]) => options?.method === "POST"),
  ).toHaveLength(1);
});

it("loads categories per destination and clears the previous selection", async () => {
  vi.mocked(api).mockImplementation(async (path) => {
    if (path === "/settings/torrents")
      return { defaultInstanceId: 0, categories: [] };
    if (path === "/qui/instances")
      return [
        { id: 7, name: "Movies client", connected: true, isActive: true },
        { id: 9, name: "TV client", connected: true, isActive: true },
      ];
    if (path === "/qui/instances/7/categories") return ["Movies"];
    if (path === "/qui/instances/9/categories") return ["TV", "TV/Anime"];
    throw new Error("Unexpected request");
  });
  render(<AddTorrent jobId="saved" result={result} />, { wrapper: Wrapper });
  fireEvent.click(screen.getByRole("button", { name: "Add to qui" }));
  expect(api).not.toHaveBeenCalledWith("/qui/instances/0/categories");
  await pick("Destination instance", "Movies client");
  await pick("Category", "Movies");
  await pick("Destination instance", "TV client");
  await openPicker("Category");
  await screen.findByRole("menuitemradio", { name: "TV/Anime" });
  expect(
    screen.getByRole("button", { name: "Category" }).textContent,
  ).toContain("No category");
  expect(screen.queryByRole("menuitemradio", { name: "Movies" })).toBeNull();
  expect(
    vi.mocked(api).mock.calls.every(([, options]) => !options?.method),
  ).toBe(true);
});

it("allows a manual category when category loading fails", async () => {
  vi.mocked(api).mockImplementation(async (path) => {
    if (path === "/settings/torrents")
      return { defaultInstanceId: 0, categories: [] };
    if (path === "/qui/instances")
      return [{ id: 7, name: "Main client", connected: true, isActive: true }];
    if (path.endsWith("/categories")) throw new Error("qui unavailable");
    return { added: 1 };
  });
  render(<AddTorrent jobId="saved" result={result} />, { wrapper: Wrapper });
  fireEvent.click(screen.getByRole("button", { name: "Add to qui" }));
  await pick("Destination instance", "Main client");
  await screen.findByText(
    "Could not load categories. Enter one manually or retry.",
  );
  fireEvent.change(screen.getByLabelText("Category"), {
    target: { value: "Movies" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Add torrent" }));
  await screen.findByText("qui accepted the torrent. Check its status in qui.");
  const post = vi
    .mocked(api)
    .mock.calls.find(([, options]) => options?.method === "POST")!;
  expect(JSON.parse(post[1]!.body as string).category).toBe("Movies");
});

it("queues an upload only after opt-in and starts the download", async () => {
  vi.mocked(api).mockImplementation(async (path) => {
    if (path === "/settings/torrents")
      return { defaultInstanceId: 0, categories: [] };
    if (path === "/qui/instances")
      return [{ id: 7, name: "Main client", connected: true, isActive: true }];
    if (path === "/settings/uploads")
      return {
        enabled: true,
        trackers: [{ indexerId: 3, code: "AITHER" }],
        paths: [{ instanceId: 7, from: "/downloads", to: "/data" }],
      };
    if (path.endsWith("/categories")) return [];
    return { added: 1, uploadJobId: "upload1" };
  });
  render(<AddTorrent jobId="saved" result={result} targetIndexerId={3} />, {
    wrapper: Wrapper,
  });
  fireEvent.click(screen.getByRole("button", { name: "Add to qui" }));
  await pick("Destination instance", "Main client");
  const toggle = await screen.findByLabelText(
    /Upload to target after download/,
  );
  expect((toggle as HTMLInputElement).checked).toBe(false);
  await waitFor(() =>
    expect((toggle as HTMLInputElement).disabled).toBe(false),
  );
  fireEvent.click(toggle);
  expect(
    (screen.getByLabelText("Add paused") as HTMLInputElement).checked,
  ).toBe(false);
  fireEvent.click(screen.getByRole("button", { name: "Add torrent" }));
  await screen.findByRole("link", { name: "View upload queue" });
  const post = vi
    .mocked(api)
    .mock.calls.find(([, options]) => options?.method === "POST")!;
  expect(JSON.parse(post[1]!.body as string)).toMatchObject({
    uploadAfterDownload: true,
    paused: false,
    instanceId: 7,
  });
});

it("restores a remembered preset and saves changed selections", async () => {
  localStorage.setItem("ravenseed.indexerPreset", "movies");
  const saved = {
    id: "movies",
    name: "Movies",
    indexerIds: [1, 2],
    targetIndexerId: 2,
  };
  vi.mocked(api).mockImplementation(async (_, options) =>
    options?.method === "PUT" ? JSON.parse(options.body as string) : [saved],
  );
  const apply = vi.fn();
  const active = vi.fn();
  render(
    <IndexerPresets
      onActiveChange={active}
      ready
      indexers={[
        { id: 1, name: "A", enabled: true, backend: "", capabilities: [] },
        { id: 2, name: "B", enabled: true, backend: "", capabilities: [] },
      ]}
      selected={[1]}
      target={0}
      apply={apply}
    />,
    { wrapper: Wrapper },
  );
  await waitFor(() => expect(apply).toHaveBeenCalledWith([1, 2], 2));
  expect(apply).toHaveBeenCalledTimes(1);
  await waitFor(() => expect(active).toHaveBeenLastCalledWith("Movies"));
  fireEvent.click(screen.getByRole("button", { name: "Update" }));
  await screen.findByText("Preset saved.");
  const put = vi
    .mocked(api)
    .mock.calls.find(([, options]) => options?.method === "PUT")!;
  expect(JSON.parse(put[1]!.body as string)).toEqual({
    ...saved,
    indexerIds: [1],
    targetIndexerId: 0,
  });
  expect(vi.mocked(api).mock.calls.some(([path]) => path === "/searches")).toBe(
    false,
  );
});

it("preselects the default instance, limits categories and sends the chosen UA preset", async () => {
  vi.mocked(api).mockImplementation(async (path) => {
    if (path === "/settings/torrents")
      return {
        defaultInstanceId: 7,
        categories: [{ instanceId: 7, enabled: ["Movies"] }],
      };
    if (path === "/qui/instances")
      return [
        { id: 7, name: "Main client", connected: true, isActive: true },
        { id: 9, name: "Other client", connected: true, isActive: true },
      ];
    if (path.endsWith("/categories")) return ["Movies", "TV"];
    if (path === "/settings/uploads")
      return {
        enabled: true,
        trackers: [{ indexerId: 3, code: "AITHER" }],
        paths: [{ instanceId: 7, from: "/downloads", to: "/data" }],
        flagPresets: [
          { id: "private", name: "Private moderation", args: "--anon --modq" },
        ],
        defaultFlagPresetId: "private",
      };
    return { added: 1, uploadJobId: "upload1" };
  });
  render(<AddTorrent jobId="saved" result={result} targetIndexerId={3} />, {
    wrapper: Wrapper,
  });
  fireEvent.click(screen.getByRole("button", { name: "Add to qui" }));
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: "Destination instance" }).textContent,
    ).toContain("Main client"),
  );
  await openPicker("Category");
  expect(screen.queryByRole("menuitemradio", { name: "TV" })).toBeNull();
  fireEvent.click(screen.getByRole("menuitemradio", { name: "Movies" }));
  await pick("Destination instance", "Other client");
  await openPicker("Category");
  expect(screen.getByRole("menuitemradio", { name: "TV" })).toBeTruthy();
  closePicker();
  await pick("Destination instance", "Main client");
  await pick("Category", "Movies");
  fireEvent.click(screen.getByLabelText(/Upload to target after download/));
  expect(
    screen.getByRole("button", { name: "UA flag preset" }).textContent,
  ).toContain("Private moderation");
  expect(screen.getByText("--anon --modq")).toBeTruthy();
  await pick("UA flag preset", "No preset");
  await pick("UA flag preset", "Private moderation");
  fireEvent.click(screen.getByRole("button", { name: "Add torrent" }));
  await screen.findByRole("link", { name: "View upload queue" });
  const post = vi
    .mocked(api)
    .mock.calls.find(([, options]) => options?.method === "POST")!;
  expect(JSON.parse(post[1]!.body as string)).toMatchObject({
    instanceId: 7,
    category: "Movies",
    uploadAfterDownload: true,
    uploadPresetId: "private",
  });
});

it("leaves an unavailable default unselected and retains restricted categories on fetch failure", async () => {
  vi.mocked(api).mockImplementation(async (path) => {
    if (path === "/settings/torrents")
      return {
        defaultInstanceId: 8,
        categories: [{ instanceId: 7, enabled: ["Movies"] }],
      };
    if (path === "/qui/instances")
      return [
        { id: 7, name: "Main client", connected: true, isActive: true },
        { id: 8, name: "Offline client", connected: false, isActive: true },
      ];
    if (path.endsWith("/categories")) throw new Error("qui unavailable");
    return { enabled: false };
  });
  render(<AddTorrent jobId="saved" result={result} />, { wrapper: Wrapper });
  fireEvent.click(screen.getByRole("button", { name: "Add to qui" }));
  await openPicker("Destination instance");
  closePicker();
  expect(
    screen.getByRole("button", { name: "Destination instance" }).textContent,
  ).toContain("Choose an instance");
  await pick("Destination instance", "Main client");
  await screen.findByText(
    "Could not load categories. Showing enabled categories from Settings.",
  );
  await pick("Category", "Movies");
  expect(
    screen.queryByPlaceholderText("Enter a category, optional"),
  ).toBeNull();
  expect(
    vi.mocked(api).mock.calls.some(([, options]) => options?.method === "POST"),
  ).toBe(false);
});
