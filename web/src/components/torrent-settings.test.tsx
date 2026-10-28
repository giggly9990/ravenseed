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
import { TorrentSettings } from "./torrent-settings";
import { api } from "../api";
import { pick, openPicker, closePicker } from "../test/pickers";

vi.mock("../api", () => ({ api: vi.fn() }));
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});

it("saves the default instance and an explicitly empty category selection", async () => {
  vi.mocked(api).mockImplementation(async (path, options) => {
    if (path === "/settings/torrents")
      return options?.method === "PUT"
        ? JSON.parse(options.body as string)
        : { defaultInstanceId: 0, categories: [] };
    if (path === "/qui/instances")
      return [{ id: 7, name: "Main client", connected: true, isActive: true }];
    return ["Movies", "TV"];
  });
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <TorrentSettings />
    </QueryClientProvider>,
  );
  await pick("Default qui instance", "Main client");
  await pick("Categories for instance", "Main client");
  const toggle = await screen.findByLabelText("Choose enabled categories");
  await waitFor(() =>
    expect((toggle as HTMLInputElement).disabled).toBe(false),
  );
  fireEvent.click(toggle);
  await openPicker("Enabled categories");
  fireEvent.click(screen.getByRole("menuitem", { name: "Clear" }));
  closePicker();
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  await screen.findByText("Torrent defaults saved.");
  const post = vi
    .mocked(api)
    .mock.calls.find(([, options]) => options?.method === "PUT")!;
  expect(JSON.parse(post[1]!.body as string)).toEqual({
    defaultInstanceId: 7,
    categories: [{ instanceId: 7, enabled: [] }],
  });
});
