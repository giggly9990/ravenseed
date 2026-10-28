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
import type { ReactNode } from "react";
import { SendToUA } from "./send-to-ua";
import { api } from "../../api";
import type { MonitorCandidate } from "../../types";

vi.mock("../../api", () => ({ api: vi.fn(), bytes: () => "1 GiB" }));
vi.mock("@tanstack/react-router", () => ({
  Link: ({ children }: { children: ReactNode }) => <a>{children}</a>,
}));
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});

const candidate: MonitorCandidate = {
  id: "cand1",
  monitorId: "mon1",
  instanceId: 7,
  hash: "c".repeat(40),
  name: "Amelie.2001.1080p.BluRay.x264-GRP",
  size: 1000,
  status: "candidate",
  message: "",
  media: { type: "movie", title: "Amelie", imdbId: "tt0211915", tmdbId: 194 },
  identity: {
    hash: "c".repeat(40),
    source: "arr_history",
    instance: "Radarr",
    media: { type: "movie", tmdbId: 194 },
    resolvedAt: "",
    expiresAt: "",
  },
  nextCheckAt: "",
  matchCount: 0,
};

function renderDialog(c: MonitorCandidate = candidate) {
  vi.mocked(api).mockImplementation(async (path) => {
    if (path === "/settings/uploads")
      return {
        enabled: true,
        trackers: [{ indexerId: 2, code: "AITHER" }],
        paths: [{ instanceId: 7, from: "/downloads", to: "/data" }],
        flagPresets: [],
      };
    return { id: "upload1", status: "downloading" };
  });
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <SendToUA
        candidate={c}
        targetIndexerId={2}
        targetName="Aither"
        close={() => {}}
      />
    </QueryClientProvider>,
  );
  return screen.getByRole("spinbutton") as HTMLInputElement;
}

async function submit() {
  const queue = screen.getByRole("button", { name: "Queue upload" });
  await waitFor(() => expect(queue.hasAttribute("disabled")).toBe(false));
  fireEvent.click(queue);
  await waitFor(() =>
    expect(
      vi.mocked(api).mock.calls.some(([path]) => path.endsWith("/upload")),
    ).toBe(true),
  );
  const call = vi
    .mocked(api)
    .mock.calls.find(([path]) => path.endsWith("/upload"))!;
  return JSON.parse(String(call[1]!.body));
}

async function send(edit: (input: HTMLInputElement) => void) {
  const input = renderDialog();
  expect(input.value).toBe("194");
  expect(screen.getByText(/From Radarr download history/)).toBeTruthy();
  edit(input);
  return submit();
}

const nameMatch: MonitorCandidate = {
  ...candidate,
  identity: { ...candidate.identity!, source: "arr_parse" },
};

it("sends the prefilled TMDb ID from download history", async () => {
  expect(await send(() => {})).toEqual({
    instanceId: 7,
    tmdbId: 194,
    uploadPresetId: "",
  });
  expect(screen.queryByRole("checkbox")).toBeNull();
});

it("clearing the prefilled ID lets UA identify the release", async () => {
  const body = await send((input) =>
    fireEvent.change(input, { target: { value: "" } }),
  );
  expect(body).toMatchObject({ tmdbId: 0, clearMediaIds: true });
});

it("keeps IMDb-only history IDs when there is no TMDb ID to clear", async () => {
  const input = renderDialog({
    ...candidate,
    media: { type: "movie", title: "Amelie", imdbId: "tt0211915" },
  });
  expect(input.value).toBe("");
  expect(await submit()).toEqual({
    instanceId: 7,
    tmdbId: 0,
    uploadPresetId: "",
  });
});

it("typing then erasing an ID without a prefill does not clear IDs", async () => {
  const input = renderDialog({
    ...candidate,
    media: { type: "movie", title: "Amelie", imdbId: "tt0211915" },
  });
  fireEvent.change(input, { target: { value: "603" } });
  fireEvent.change(input, { target: { value: "" } });
  expect(await submit()).not.toHaveProperty("clearMediaIds");
});

it("names the target tracker in the readiness checks", async () => {
  renderDialog();
  expect(
    await screen.findByText("Aither is mapped to a UA tracker code"),
  ).toBeTruthy();
  expect(screen.queryByText(/Tracker B/)).toBeNull();
});

it("requires confirming library name-match IDs before sending", async () => {
  const input = renderDialog(nameMatch);
  expect(input.value).toBe("194");
  expect(
    screen.getByText(/Radarr library name match, not download history/),
  ).toBeTruthy();
  await screen.findByText("Aither is mapped to a UA tracker code");
  const queue = screen.getByRole("button", { name: "Queue upload" });
  expect(queue.hasAttribute("disabled")).toBe(true);
  fireEvent.click(screen.getByRole("checkbox", { name: /Confirm these IDs/ }));
  expect(await submit()).toEqual({
    instanceId: 7,
    tmdbId: 194,
    confirmMediaIds: true,
    uploadPresetId: "",
  });
});

it("a replaced name-match ID needs no confirmation", async () => {
  const input = renderDialog(nameMatch);
  fireEvent.change(input, { target: { value: "603" } });
  expect(screen.queryByRole("checkbox")).toBeNull();
  expect(await submit()).toEqual({
    instanceId: 7,
    tmdbId: 603,
    uploadPresetId: "",
  });
});

it("a cleared name-match ID clears without confirmation", async () => {
  const input = renderDialog(nameMatch);
  fireEvent.change(input, { target: { value: "" } });
  expect(await submit()).toEqual({
    instanceId: 7,
    tmdbId: 0,
    clearMediaIds: true,
    uploadPresetId: "",
  });
});
