// @vitest-environment jsdom
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { createMemoryHistory, RouterProvider } from "@tanstack/react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { api } from "../../api";
import { router } from "../../app/router";
import type { Settings } from "../../types";

vi.mock("../../api", async (original) => ({
  ...(await original<typeof import("../../api")>()),
  api: vi.fn(),
}));
afterEach(cleanup);

it("keeps connection, upload and matching drafts when another section saves", async () => {
  vi.spyOn(window, "scrollTo").mockImplementation(() => {});
  const settings: Settings = {
    quiUrl: "http://saved-qui:7476",
    hasApiKey: true,
    defaultMinCoverage: 1,
    defaultTargetIndexer: 0,
    environmentManaged: false,
    metadata: { language: "en-US", hasToken: true, environmentManaged: false },
    matching: {
      matchAlternateTitles: true,
      sizeTolerancePercent: 0.1,
      groupAliases: {},
      ignoredTags: [],
      episodeMappings: [],
    },
  };
  const uploads = {
    enabled: false,
    url: "http://saved-ua:5000",
    hasToken: true,
    trackers: [],
    paths: [],
    flagPresets: [],
  };
  vi.mocked(api).mockImplementation(async (path, options) => {
    if (path === "/settings") {
      if (options?.method === "PUT")
        Object.assign(settings, JSON.parse(String(options.body)), {
          apiKey: undefined,
        });
      return structuredClone(settings);
    }
    if (path === "/settings/metadata") {
      settings.metadata!.language = JSON.parse(String(options?.body)).language;
      return structuredClone(settings);
    }
    if (path === "/settings/uploads") return structuredClone(uploads);
    if (path === "/settings/arr") return { instances: [] };
    if (path === "/settings/torrents")
      return { defaultInstanceId: 0, categories: [] };
    if (path === "/indexers" || path === "/qui/instances") return [];
    if (path === "/session") return { enabled: false, authenticated: false };
    if (path === "/ready") return { status: "ok", qui: "connected" };
    throw new Error(`Unexpected request: ${path}`);
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  router.update({
    history: createMemoryHistory({ initialEntries: ["/settings"] }),
  });
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  await act(async () => {
    await router.load();
  });
  const tab = (name: RegExp) =>
    fireEvent.click(screen.getByRole("tab", { name }));
  await waitFor(() =>
    expect((screen.getByLabelText("qui URL") as HTMLInputElement).value).toBe(
      settings.quiUrl,
    ),
  );
  fireEvent.change(screen.getByLabelText("qui URL"), {
    target: { value: "http://draft-qui:7476" },
  });
  expect((screen.getByLabelText("API key") as HTMLInputElement).type).toBe(
    "password",
  );
  expect(
    screen.getByLabelText("API key").closest(".secret-input"),
  ).toBeTruthy();
  fireEvent.change(screen.getByLabelText("API key"), {
    target: { value: "draft-api-key" },
  });
  tab(/Upload-Assistant/);
  expect(router.state.location.search).toEqual({ section: "uploads" });
  fireEvent.change(screen.getByLabelText("WebUI URL"), {
    target: { value: "http://draft-ua:5000" },
  });
  tab(/Matching rules/);
  fireEvent.change(screen.getByLabelText(/Size tolerance/), {
    target: { value: "0.5" },
  });
  expect(
    screen.getByRole("tab", { name: /Upload-Assistant, unsaved changes/ }),
  ).toBeTruthy();
  expect(
    screen.getByRole("tab", { name: /^qui, unsaved changes/ }),
  ).toBeTruthy();
  tab(/TMDb metadata/);
  fireEvent.change(screen.getByLabelText("Metadata language"), {
    target: { value: "fr-FR" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  await screen.findByText("TMDb settings saved.");
  tab(/^qui/);
  expect((screen.getByLabelText("qui URL") as HTMLInputElement).value).toBe(
    "http://draft-qui:7476",
  );
  expect((screen.getByLabelText("API key") as HTMLInputElement).value).toBe(
    "draft-api-key",
  );
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  await screen.findByText("Settings saved. Your connection is ready.");
  expect((screen.getByLabelText("API key") as HTMLInputElement).value).toBe("");
  await act(async () => {
    await client.invalidateQueries();
  });
  tab(/Upload-Assistant/);
  expect((screen.getByLabelText("WebUI URL") as HTMLInputElement).value).toBe(
    "http://draft-ua:5000",
  );
  tab(/Matching rules/);
  expect(
    (screen.getByLabelText(/Size tolerance/) as HTMLInputElement).value,
  ).toBe("0.5");
  client.clear();
});

it("opens the section named in ?section= and replaces it on switch", async () => {
  vi.mocked(api).mockImplementation(async (path) => {
    if (path === "/settings")
      return {
        quiUrl: "http://qui",
        hasApiKey: false,
        defaultMinCoverage: 1,
        defaultTargetIndexer: 0,
        environmentManaged: false,
      };
    if (path === "/settings/uploads")
      return {
        enabled: false,
        url: "",
        hasToken: false,
        trackers: [],
        paths: [],
      };
    if (path === "/settings/torrents")
      return { defaultInstanceId: 0, categories: [] };
    if (path === "/session") return { enabled: false, authenticated: false };
    if (path === "/ready") return { status: "ok", qui: "connected" };
    if (path.startsWith("/uploads")) return { items: [], total: 0 };
    return [];
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const history = createMemoryHistory({
    initialEntries: ["/", "/settings?section=uploads"],
  });
  router.update({ history });
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  await act(async () => {
    await router.load();
  });
  await screen.findByRole("heading", { name: "Upload-Assistant" });
  expect(
    screen
      .getByRole("tab", { name: "Upload-Assistant" })
      .getAttribute("aria-selected"),
  ).toBe("true");
  const before = history.length;
  fireEvent.click(screen.getByRole("tab", { name: "Matching rules" }));
  await waitFor(() =>
    expect(router.state.location.search).toEqual({ section: "matching" }),
  );
  expect(history.length).toBe(before);
  expect(screen.getByRole("heading", { name: "Matching rules" })).toBeTruthy();
  client.clear();
});
