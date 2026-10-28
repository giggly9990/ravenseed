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
import { IndexerSearchSettings } from "./indexer-search-settings";
import { api } from "../api";
import type { Settings } from "../types";
import { pick } from "../test/pickers";
vi.mock("../api", async (original) => ({
  ...(await original<typeof import("../api")>()),
  api: vi.fn(),
}));
afterEach(cleanup);
it("saves separate movie and TV methods and resets to Auto", async () => {
  const settings: Settings = {
    quiUrl: "http://qui",
    hasApiKey: true,
    defaultTargetIndexer: 0,
    defaultMinCoverage: 2,
    environmentManaged: false,
  };
  const writes: unknown[] = [];
  vi.mocked(api).mockImplementation(async (path, options) => {
    if (path === "/indexers")
      return [
        { id: 1, name: "Tracker A", enabled: true },
        { id: 2, name: "Tracker B", enabled: true },
      ];
    if (path === "/settings/search-overrides") {
      const value = JSON.parse(String(options?.body));
      writes.push(value);
      return { ...settings, searchOverrides: value };
    }
    throw new Error(path);
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <IndexerSearchSettings settings={settings} />
    </QueryClientProvider>,
  );
  await screen.findByText("Tracker A");
  await pick("Tracker A movies search method", "Title");
  await pick("Tracker A TV search method", "TVDb");
  await pick("Tracker B movies search method", "TMDb");
  expect(
    screen.getByRole("region", { name: "Unsaved changes" }).textContent,
  ).toContain("2 unsaved changes");
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() =>
    expect(writes).toEqual([
      { 1: { movie: "title", tv: "tvdb" }, 2: { movie: "tmdb", tv: "auto" } },
    ]),
  );
  await screen.findByRole("status");
  expect(screen.getAllByRole("columnheader").map((h) => h.textContent)).toEqual(
    ["Indexer", "Movies", "TV"],
  );
  fireEvent.click(screen.getByRole("button", { name: "Reset all to Auto" }));
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() => expect(writes[1]).toEqual({}));
});

it("drops an indexer's override once both methods are back on Auto", async () => {
  const settings: Settings = {
    quiUrl: "http://qui",
    hasApiKey: true,
    defaultTargetIndexer: 0,
    defaultMinCoverage: 2,
    environmentManaged: false,
    searchOverrides: { 1: { movie: "title", tv: "auto" } },
  };
  const writes: unknown[] = [];
  vi.mocked(api).mockImplementation(async (path, options) => {
    if (path === "/indexers")
      return [{ id: 1, name: "Tracker A", enabled: true }];
    if (path === "/settings/search-overrides") {
      const value = JSON.parse(String(options?.body));
      writes.push(value);
      return { ...settings, searchOverrides: value };
    }
    throw new Error(path);
  });
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <IndexerSearchSettings settings={settings} />
    </QueryClientProvider>,
  );
  await screen.findByText("Tracker A");
  expect(screen.getByText("Override")).toBeTruthy();
  await pick("Tracker A movies search method", "Auto");
  expect(screen.queryByText("Override")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() => expect(writes).toEqual([{}]));
});
