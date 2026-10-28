// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { UploadSettings } from "./upload-settings";
import { api } from "../api";
import { pick } from "../test/pickers";

vi.mock("../api", () => ({ api: vi.fn() }));
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});

function renderSettings() {
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <UploadSettings />
    </QueryClientProvider>,
  );
}

const putBody = () => {
  const put = vi
    .mocked(api)
    .mock.calls.filter(([, opts]) => opts?.method === "PUT")
    .at(-1)!;
  expect(put[0]).toBe("/settings/uploads");
  return JSON.parse(put[1]!.body as string);
};

it("creates a freeform UA preset, sets its default and removes it", async () => {
  const saved = {
    enabled: false,
    url: "http://ua:5000",
    hasToken: true,
    trackers: [],
    paths: [],
  };
  vi.mocked(api).mockImplementation(async (path, options) =>
    path === "/settings/uploads"
      ? options?.method === "PUT"
        ? JSON.parse(options.body as string)
        : saved
      : [],
  );
  renderSettings();
  await screen.findByDisplayValue("http://ua:5000");
  fireEvent.click(screen.getByRole("button", { name: "+ Add preset" }));
  fireEvent.change(screen.getByLabelText("Preset name 1"), {
    target: { value: "Private moderation" },
  });
  fireEvent.change(screen.getByLabelText("UA flags for Private moderation"), {
    target: { value: "--anon --modq" },
  });
  fireEvent.click(
    screen.getByRole("radio", { name: "Use Private moderation as default" }),
  );
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  await screen.findByText("Upload settings saved.");
  const value = putBody();
  expect(value.flagPresets).toEqual([
    {
      id: value.defaultFlagPresetId,
      name: "Private moderation",
      args: "--anon --modq",
    },
  ]);
  fireEvent.click(
    screen.getByRole("button", { name: "Remove Private moderation" }),
  );
  expect(screen.queryByLabelText("UA flags for Private moderation")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  await screen.findByText("Upload settings saved.");
  expect(putBody()).toMatchObject({ flagPresets: [], defaultFlagPresetId: "" });
});

it("saves connection settings independently and clears the credential input", async () => {
  const saved = {
    enabled: false,
    url: "http://ua:5000",
    hasToken: true,
    trackers: [],
    paths: [],
  };
  vi.mocked(api).mockImplementation(async (path, options) =>
    path === "/settings/uploads"
      ? { ...saved, enabled: options?.method === "PUT" }
      : [],
  );
  renderSettings();
  await screen.findByDisplayValue("http://ua:5000");
  const token = screen.getByLabelText("API token") as HTMLInputElement;
  // SecretInput: a password field with a Show/Hide button.
  expect(token.type).toBe("password");
  expect(token.placeholder).toBe("Saved · leave blank to keep");
  fireEvent.click(screen.getByRole("button", { name: "Show value" }));
  expect(token.type).toBe("text");
  expect(screen.getByLabelText("WebUI URL").dataset.urlField).toBe("true");
  fireEvent.change(token, { target: { value: "private-token" } });
  const queue = screen.getByRole("switch", { name: "Upload queue" });
  expect(queue.getAttribute("aria-checked")).toBe("false");
  fireEvent.click(queue);
  expect(
    screen.getByRole("region", { name: "Unsaved changes" }).textContent,
  ).toContain("2 unsaved changes");
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  await screen.findByText("Upload settings saved.");
  expect(token.value).toBe("");
  expect(putBody()).toMatchObject({ token: "private-token", enabled: true });
  expect(screen.queryByRole("region", { name: "Unsaved changes" })).toBeNull();
});

it("edits tracker and path rows with accessible labels and removes the token", async () => {
  const saved = {
    enabled: true,
    url: "http://ua:5000",
    hasToken: true,
    trackers: [{ indexerId: 2, code: "ORB" }],
    paths: [],
  };
  vi.mocked(api).mockImplementation(async (path, options) => {
    if (path === "/settings/uploads")
      return options?.method === "PUT"
        ? JSON.parse(options.body as string)
        : saved;
    if (path === "/indexers")
      return [
        { id: 2, name: "Orbit", enabled: true },
        { id: 3, name: "Atlas", enabled: true },
      ];
    if (path === "/qui/instances")
      return [{ id: 1, name: "Homelab", connected: true, isActive: true }];
    return [];
  });
  renderSettings();
  const code = await screen.findByLabelText("Tracker code for Orbit");
  fireEvent.change(code, { target: { value: "orbx" } });
  fireEvent.click(screen.getByRole("button", { name: "+ Add tracker" }));
  await pick("Indexer for tracker row 2", "Atlas");
  fireEvent.change(screen.getByLabelText("Tracker code for Atlas"), {
    target: { value: "atl" },
  });
  fireEvent.click(screen.getByRole("button", { name: "+ Add path" }));
  await pick("qui instance for path row 1", "Homelab");
  fireEvent.change(screen.getByLabelText("qBittorrent path for Homelab"), {
    target: { value: "/downloads" },
  });
  fireEvent.change(screen.getByLabelText("Upload-Assistant path for Homelab"), {
    target: { value: "/data" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Remove saved token" }));
  expect(
    screen
      .getByRole("switch", { name: "Upload queue" })
      .getAttribute("aria-checked"),
  ).toBe("false");
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  await screen.findByText("Upload settings saved.");
  expect(putBody()).toMatchObject({
    clearToken: true,
    enabled: false,
    trackers: [
      { indexerId: 2, code: "ORBX" },
      { indexerId: 3, code: "ATL" },
    ],
    paths: [{ instanceId: 1, from: "/downloads", to: "/data" }],
  });
});

it("discards edits and shows a status pill after testing", async () => {
  const saved = {
    enabled: false,
    url: "http://ua:5000",
    hasToken: false,
    trackers: [],
    paths: [],
  };
  vi.mocked(api).mockImplementation(async (path) =>
    path.startsWith("/settings/uploads") ? saved : [],
  );
  renderSettings();
  const url = (await screen.findByDisplayValue(
    "http://ua:5000",
  )) as HTMLInputElement;
  expect(screen.queryByText(/Reachable/)).toBeNull();
  fireEvent.change(url, { target: { value: "http://other:5000" } });
  fireEvent.click(screen.getByRole("button", { name: "Discard" }));
  expect(url.value).toBe("http://ua:5000");
  fireEvent.click(screen.getByRole("button", { name: "Test connection" }));
  expect(await screen.findByText(/Reachable · tested just now/)).toBeTruthy();
  expect(api).toHaveBeenCalledWith(
    "/settings/uploads/test",
    expect.objectContaining({ method: "POST" }),
  );
});
