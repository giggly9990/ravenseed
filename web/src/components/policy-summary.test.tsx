// @vitest-environment jsdom
import { afterEach, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { PolicySummary } from "./policy-summary";
import type { PolicyEvaluation } from "../types";

afterEach(cleanup);

const finding: PolicyEvaluation = {
  classification: "equivalent",
  reason: "Equivalent release found",
  automaticEvidence: [],
  missingEvidence: [],
  reviewReasons: [],
  competingResultIds: ["5a82b2604597cff00e888009"],
  improvements: [],
  regressions: [],
  evaluatedAt: "2026-09-25T00:00:00Z",
};

it("shows the target release name and tracker instead of only its ID", () => {
  render(
    <PolicySummary
      evaluation={{
        ...finding,
        targetResults: [
          {
            id: "5a82b2604597cff00e888009",
            title: "Example.2024.2160p.WEB-DL-GROUP",
            indexer: "Target Tracker",
            size: 1073741824,
            infoUrl: "https://tracker.example/torrents/123",
            relation: "equivalent",
          },
        ],
      }}
    />,
  );
  expect(
    screen.getByRole("link", { name: "Example.2024.2160p.WEB-DL-GROUP" }),
  ).toHaveProperty("href", "https://tracker.example/torrents/123");
  expect(screen.getByText(/Target Tracker/)).toBeTruthy();
  expect(screen.getByText(/Result ID 5a82b2604597cff00e888009/)).toBeTruthy();
});

it("explains how to refresh an older ID-only finding", () => {
  render(<PolicySummary evaluation={finding} />);
  expect(screen.getByText(/Run a fresh search or Check again/)).toBeTruthy();
});

it("keeps the one-line compact form and adds an inline why form", () => {
  const trump: PolicyEvaluation = {
    ...finding,
    classification: "trump",
    reason: "Better audio",
    improvements: [{ type: "audio", from: "5.1", to: "7.1" }],
    regressions: [{ type: "hdr", from: "DV", to: "HDR10" }],
    targetResults: [
      {
        id: "t1",
        title: "Old.Release-GRP",
        indexer: "Orbit",
        size: 1073741824,
        relation: "same_slot",
      },
    ],
  };
  const { container, unmount } = render(
    <PolicySummary evaluation={trump} compact />,
  );
  expect(container.textContent).toContain("Target policy: trump");
  expect(container.textContent).not.toContain("Old.Release-GRP");
  unmount();
  render(<PolicySummary evaluation={trump} why />);
  expect(screen.getByText("Trump")).toBeTruthy();
  expect(screen.getByLabelText("Improvement")).toBeTruthy();
  expect(screen.getByLabelText("Regression")).toBeTruthy();
  expect(screen.getByText("Old.Release-GRP")).toBeTruthy();
  expect(screen.queryByText(/Result ID/)).toBeNull();
});
