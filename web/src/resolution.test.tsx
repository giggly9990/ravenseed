// @vitest-environment jsdom
import { expect, it, vi } from "vitest";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { createMemoryHistory, RouterProvider } from "@tanstack/react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { api } from "./api";
import { pick, openPicker, closePicker } from "./test/pickers";

vi.mock("./api", async (original) => ({
  ...(await original<typeof import("./api")>()),
  api: vi.fn(),
}));
vi.mock("react-dom/client", async (original) => {
  const module = await original<typeof import("react-dom/client")>();
  return {
    ...module,
    createRoot: (
      element: Element,
      options: Parameters<typeof module.createRoot>[1],
    ) => (element ? module.createRoot(element, options) : { render: () => {} }),
  };
});

it("shows a saved search, filters results and refreshes without a launch overlay", async () => {
  vi.spyOn(window, "scrollTo").mockImplementation(() => {});
  const job = {
    id: "saved",
    status: "completed",
    request: {
      media: { type: "movie", imdbId: "tt0133093" },
      indexerIds: [1],
      targetIndexerId: 1,
      filters: { minimumCoverage: 1 },
    },
    startedAt: "2026-09-23T00:00:00Z",
    indexers: [
      { indexerId: 1, name: "Atlas", status: "completed", resultCount: 2 },
    ],
    indexerErrors: [],
    resultCount: 2,
    familyCount: 2,
    candidateCount: 1,
  };
  vi.mocked(api).mockImplementation(async (path) => {
    if (path === "/settings")
      return {
        quiUrl: "http://qui",
        hasApiKey: true,
        defaultMinCoverage: 1,
        defaultTargetIndexer: 0,
      };
    if (path === "/indexers")
      return [
        { id: 1, name: "Atlas", enabled: true, backend: "", capabilities: [] },
      ];
    if (path === "/presets") return [];
    if (path === "/session") return { enabled: false, authenticated: false };
    if (path === "/ready") return { status: "ok", qui: "connected" };
    if (path === "/searches/saved") return job;
    if (path === "/searches") return job;
    if (path === "/media/resolve") return {};
    if (
      path.startsWith("/searches/saved/families?") ||
      path.startsWith("/searches/saved/candidates?")
    )
      return { items: [], total: 0, limit: 20, offset: 0 };
    throw new Error(`Unexpected request: ${path}`);
  });
  const { router } = await import("./main");
  router.update({
    history: createMemoryHistory({
      initialEntries: ["/?job=saved&resolution=2160p"],
    }),
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  await act(async () => {
    await router.load();
  });
  await waitFor(() =>
    expect(screen.getAllByRole("button", { name: "Resolution" })).toHaveLength(
      2,
    ),
  );
  // The saved request is shown in the always-visible search row.
  const identifier = screen.getByRole("textbox", {
    name: "Content identifier",
  }) as HTMLInputElement;
  await waitFor(() => expect(identifier.value).toBe("tt0133093"));
  fireEvent.change(identifier, { target: { value: "invalid-draft" } });
  const [before, results] = screen.getAllByRole("button", {
    name: "Resolution",
  });
  expect(before.textContent).toContain("2160p");
  expect(results.textContent).toContain("2160p");
  await openPicker(results);
  fireEvent.click(screen.getByRole("menuitem", { name: "Clear" }));
  await waitFor(() =>
    expect(
      screen
        .getByRole("menuitemcheckbox", { name: "2160p" })
        .getAttribute("aria-checked"),
    ).toBe("false"),
  );
  fireEvent.click(screen.getByRole("menuitemcheckbox", { name: "1080p" }));
  closePicker();
  await waitFor(() => expect(before.textContent).toContain("1080p"));
  await waitFor(() =>
    expect(
      vi
        .mocked(api)
        .mock.calls.some(([path]) => path.includes("resolution=1080p")),
    ).toBe(true),
  );
  await pick(before, "Any resolution");
  await waitFor(() => expect(results.textContent).not.toContain("1080p"));
  await openPicker(results);
  fireEvent.click(screen.getByRole("menuitemcheckbox", { name: "2160p" }));
  await waitFor(() =>
    expect(
      screen
        .getByRole("menuitemcheckbox", { name: "2160p" })
        .getAttribute("aria-checked"),
    ).toBe("true"),
  );
  fireEvent.click(screen.getByRole("menuitemcheckbox", { name: "1080p" }));
  closePicker();
  await waitFor(() => expect(before.textContent).toContain("2160p + 1080p"));
  await pick("Sort direction", "Ascending");
  await waitFor(() =>
    expect(
      vi.mocked(api).mock.calls.some(([path]) => path.includes("order=asc")),
    ).toBe(true),
  );
  expect(
    vi.mocked(api).mock.calls.some(([, options]) => options?.method === "POST"),
  ).toBe(false);
  expect(router.state.location.search.job).toBe("saved");
  expect(screen.getByText("xD")).toBeTruthy();
  // got coke? appears once for a job with opportunities and opens that tab.
  const cue = await screen.findByText("got coke?");
  expect(cue.parentElement?.textContent).toContain("1 opportunity on Atlas");
  fireEvent.click(screen.getByRole("button", { name: "show them" }));
  expect(
    screen
      .getByRole("tab", { name: /Opportunities/ })
      .getAttribute("aria-selected"),
  ).toBe("true");
  await screen.findByText("No opportunities found");
  expect(screen.queryByText("got coke?")).toBeNull();
  fireEvent.click(screen.getByRole("tab", { name: /All families/ }));
  fireEvent.click(screen.getByRole("tab", { name: /Opportunities/ }));
  expect(screen.queryByText("got coke?")).toBeNull();
  fireEvent.click(screen.getByRole("tab", { name: /All families/ }));
  await pick("Sort direction", "Descending");
  expect(identifier.value).toBe("invalid-draft");
  fireEvent.submit(identifier.closest("form")!);
  expect(await screen.findByText(/Enter a valid IMDb ID/)).toBeTruthy();
  fireEvent.change(identifier, { target: { value: "tt0133093" } });
  fireEvent.submit(identifier.closest("form")!);
  expect(screen.getByText("spread the seed")).toBeTruthy();
  await waitFor(() => expect(screen.queryByText("spread the seed")).toBeNull());
  expect(
    vi
      .mocked(api)
      .mock.calls.filter(
        ([path, options]) => path === "/searches" && options?.method === "POST",
      ),
  ).toHaveLength(1);
  const implementation = vi.mocked(api).getMockImplementation()!;
  vi.mocked(api).mockImplementation(async (path, options) => {
    if (path === "/searches") throw new Error("Search refresh unavailable");
    return implementation(path, options);
  });
  fireEvent.click(screen.getByRole("button", { name: "Refresh this search" }));
  await screen.findByText("Search refresh unavailable");
  expect(screen.getByRole("alert").textContent).toContain("xD");
  expect(screen.queryByText("spread the seed")).toBeNull();
  const refreshPost = vi
    .mocked(api)
    .mock.calls.filter(
      ([path, options]) => path === "/searches" && options?.method === "POST",
    )
    .at(-1)!;
  expect(JSON.parse(refreshPost[1]!.body as string)).toMatchObject({
    refresh: true,
    indexerIds: [1],
  });
  await act(async () => {
    await router.navigate({ to: "/", search: {} });
  });
  expect(
    screen.getByRole("textbox", { name: "Content identifier" }),
  ).toBeTruthy();
  expect(
    screen.queryByRole("button", { name: "Refresh this search" }),
  ).toBeNull();
  expect(screen.getByRole("link", { name: /Recent searches/ })).toBeTruthy();
}, 20_000); // Renders the whole app; slow when the suite runs files in parallel.
