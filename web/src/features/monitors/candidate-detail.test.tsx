// @vitest-environment jsdom
import { afterEach, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router";
import { act } from "react";
import { CandidateDetail } from "./candidate-detail";
import type { Monitor, MonitorCandidate } from "../../types";

afterEach(cleanup);

const monitor = {
  id: "mon1",
  sourceIndexerName: "A",
  targetIndexerName: "B",
  enabled: true,
} as Monitor;
const candidate = {
  id: "cand1",
  monitorId: "mon1",
  instanceId: 7,
  hash: "a".repeat(40),
  name: "The.Matrix.1999.1080p.WEB-DL-NTb",
  size: 1000,
  status: "candidate",
  media: { type: "movie", title: "The Matrix" },
  nextCheckAt: new Date().toISOString(),
} as MonitorCandidate;

async function renderDetail(c: MonitorCandidate) {
  const router = createRouter({
    routeTree: createRootRoute({
      component: () => (
        <CandidateDetail
          candidate={c}
          monitor={monitor}
          close={() => {}}
          send={() => {}}
          check={() => {}}
          checking={false}
        />
      ),
    }),
    history: createMemoryHistory({ initialEntries: ["/"] }),
  });
  render(<RouterProvider router={router} />);
  await act(async () => {
    await router.load();
  });
}

it("reports a failed Sonarr/Radarr lookup instead of 'not found'", async () => {
  await renderDetail({ ...candidate, identityError: "Sonarr: timeout" });
  expect(screen.getByText(/lookup failed; searched by title/)).toBeTruthy();
  expect(screen.queryByText(/Not found in Sonarr or Radarr/)).toBeNull();
  expect(screen.getByText("Sonarr: timeout")).toBeTruthy();
});
