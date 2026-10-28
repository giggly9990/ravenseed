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
import { ArrSettings } from "./arr-settings";
import { api } from "../api";

vi.mock("../api", () => ({ api: vi.fn() }));
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});

const saved = {
  instances: [
    {
      id: "arr_1",
      name: "Sonarr",
      kind: "sonarr",
      url: "http://sonarr:8989",
      hasApiKey: true,
      enabled: true,
    },
  ],
};

function renderSettings() {
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <ArrSettings />
    </QueryClientProvider>,
  );
}

it("adds a Radarr instance and keeps the saved Sonarr key blank", async () => {
  vi.mocked(api).mockImplementation(async (path, options) => {
    if (path === "/settings/arr" && options?.method === "PUT")
      return JSON.parse(String(options.body));
    if (path === "/settings/arr") return structuredClone(saved);
    if (path === "/settings/arr/test") return { connected: true };
    throw new Error(`Unexpected request: ${path}`);
  });
  renderSettings();
  expect(
    await screen.findByPlaceholderText("Saved · leave blank to keep"),
  ).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "+ Add Radarr" }));
  fireEvent.change(screen.getAllByRole("textbox", { name: /URL/ })[1], {
    target: { value: "http://radarr:7878" },
  });
  fireEvent.change(
    screen.getByPlaceholderText("Settings > General > API Key"),
    { target: { value: "radarr-key" } },
  );
  fireEvent.click(
    screen.getAllByRole("button", { name: "Test connection" })[1],
  );
  expect(await screen.findByText("Connected")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() =>
    expect(
      vi
        .mocked(api)
        .mock.calls.some(
          ([path, opts]) => path === "/settings/arr" && opts?.method === "PUT",
        ),
    ).toBe(true),
  );
  const put = vi
    .mocked(api)
    .mock.calls.find(([, opts]) => opts?.method === "PUT")!;
  const body = JSON.parse(String(put[1]!.body));
  expect(body.instances).toHaveLength(2);
  expect(body.instances[0]).toMatchObject({ id: "arr_1", apiKey: "" });
  expect(body.instances[1]).toMatchObject({
    kind: "radarr",
    url: "http://radarr:7878",
    apiKey: "radarr-key",
    enabled: true,
  });
  const test = vi
    .mocked(api)
    .mock.calls.find(([path]) => path === "/settings/arr/test")!;
  expect(JSON.parse(String(test[1]!.body)).apiKey).toBe("radarr-key");
});

it("keeps the form locked until the saved instances load, so a save can't drop them", async () => {
  let resolve: (value: unknown) => void = () => {};
  vi.mocked(api).mockImplementation(
    (path) =>
      new Promise((done, fail) => {
        if (path === "/settings/arr") resolve = done;
        else fail(new Error(`Unexpected request: ${path}`));
      }),
  );
  renderSettings();
  const add = screen.getByRole("button", { name: "+ Add Radarr" });
  expect(add.matches(":disabled")).toBe(true);
  resolve(structuredClone(saved));
  await waitFor(() => expect(add.matches(":disabled")).toBe(false));
  expect(screen.getByRole("group", { name: "Sonarr" })).toBeTruthy();
});

it("stays locked and reports the error when the saved instances fail to load", async () => {
  vi.mocked(api).mockRejectedValue(new Error("Database unavailable"));
  renderSettings();
  expect(await screen.findByText("Database unavailable")).toBeTruthy();
  expect(
    screen.getByRole("button", { name: "+ Add Sonarr" }).matches(":disabled"),
  ).toBe(true);
});
