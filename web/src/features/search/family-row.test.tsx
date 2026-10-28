// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { FamilyTable, targetPill } from "./family-row";
import type { Family, PolicyEvaluation, Result } from "../../types";

afterEach(cleanup);

function wrapper({ children }: { children: ReactNode }) {
  return (
    <QueryClientProvider client={new QueryClient()}>
      {children}
    </QueryClientProvider>
  );
}

const parsed = {
  mediaType: "movie",
  title: "Example",
  year: 2024,
  season: 0,
  episode: 0,
  structure: "movie",
  resolution: "2160p",
  releaseType: "REMUX",
  source: "UHD_BLURAY",
  videoCodec: "HEVC",
  bitDepth: "10",
  hdr: ["HDR10"],
  audioCodec: "DTS-HD MA",
  audioChannels: "7.1",
  edition: [],
  cut: [],
  group: "GROUP",
  displayGroup: "GrOuP",
  repack: false,
  proper: false,
};
const result = (id: string, indexerId: number, indexer: string) =>
  ({
    id,
    indexerId,
    indexer,
    title: `Example.2024.2160p.REMUX-GrOuP.${indexer}`,
    size: 1073741824,
    seeders: indexerId * 10,
    infoUrl: `https://${indexer.toLowerCase()}.example/t/1`,
    hasDownloadUrl: true,
    raw: { download_volume_factor: indexerId === 1 ? 0 : 1 },
    parsed,
    confidence: "EXACT",
    evidence: [],
  }) as unknown as Result;
const trump: PolicyEvaluation = {
  classification: "trump",
  reason: "Configured objective improvement over observed target slot",
  automaticEvidence: [],
  missingEvidence: [],
  reviewReasons: [],
  competingResultIds: ["t1"],
  targetResults: [
    {
      id: "t1",
      title: "Example.2024.2160p.REMUX-OLD",
      indexer: "Orbit",
      size: 2147483648,
      relation: "policy_competitor",
    },
  ],
  improvements: [{ type: "audio", from: "5.1", to: "7.1" }],
  regressions: [],
  evaluatedAt: "2026-10-22T00:00:00Z",
};
const status = {
  indexerId: 9,
  evaluated: true,
  candidate: true,
  exactMatchFound: false,
  relatedMatchFound: true,
  message: "No equivalent result found",
};
const family = {
  id: "fam1",
  canonical: parsed,
  results: [result("a", 1, "Atlas"), result("b", 2, "Beacon")],
  variants: [
    {
      size: 1073741824,
      coverage: 2,
      indexerIds: [1, 2],
      resultIds: ["a", "b"],
      confidence: "EXACT",
      targetStatus: status,
      policy: trump,
    },
  ],
  coverage: 2,
  totalCoverage: 2,
  confidence: "EXACT",
  evidence: [],
  seeders: 30,
  targetStatus: status,
} as Family;

it("derives the target pill from the status and policy", () => {
  expect(targetPill(family)).toEqual({ tone: "accent", text: "Trump" });
  expect(
    targetPill({
      ...family,
      variants: [{ ...family.variants[0], policy: undefined }],
      targetStatus: { ...status, candidate: false, exactMatchFound: true },
    }),
  ).toEqual({ tone: "ok", text: "On target" });
  expect(
    targetPill({
      ...family,
      variants: [
        {
          ...family.variants[0],
          policy: { ...trump, classification: "coexistence" },
        },
      ],
    }).tone,
  ).toBe("warn");
  expect(
    targetPill({
      ...family,
      targetStatus: { ...status, evaluated: false },
    }),
  ).toEqual({ tone: "quiet", text: "Not evaluated" });
});

it("expands a family inline with the why box, listings and details", () => {
  const onDetails = vi.fn();
  render(
    <FamilyTable
      jobId="job1"
      families={[family]}
      selected={3}
      targetName="Orbit"
      onDetails={onDetails}
    />,
    { wrapper },
  );
  expect(screen.getByRole("columnheader", { name: "Orbit" })).toBeTruthy();
  expect(screen.getByText("Trump").className).toContain("pill accent");
  expect(screen.getByText("Freeleech")).toBeTruthy();
  const toggle = screen.getByRole("button", {
    name: "Expand 2160p UHD BluRay Remux GrOuP",
  });
  expect(toggle.getAttribute("aria-expanded")).toBe("false");
  expect(screen.queryByRole("button", { name: "Add to qui" })).toBeNull();
  fireEvent.click(toggle);
  expect(toggle.getAttribute("aria-expanded")).toBe("true");
  const why = screen.getByRole("region", { name: "Why" });
  expect(why.textContent).toContain("Against Orbit");
  expect(why.textContent).toContain("5.1 → 7.1");
  expect(why.textContent).toContain("Example.2024.2160p.REMUX-OLD");
  expect(screen.getAllByRole("button", { name: "Add to qui" })).toHaveLength(2);
  fireEvent.click(
    screen.getByRole("button", { name: "Compare & raw evidence" }),
  );
  expect(onDetails).toHaveBeenCalledWith(family);
  fireEvent.click(toggle);
  expect(screen.queryByRole("region", { name: "Why" })).toBeNull();
});
