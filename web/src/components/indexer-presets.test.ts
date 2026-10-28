import { expect, it } from "vitest";
import { availablePreset } from "./indexer-presets";

it("restores available preset indexers and clears an unavailable target", () => {
  expect(
    availablePreset(
      { id: "p", name: "Movies", indexerIds: [1, 2, 3], targetIndexerId: 2 },
      [
        { id: 1, name: "A", enabled: true, backend: "", capabilities: [] },
        { id: 2, name: "B", enabled: false, backend: "", capabilities: [] },
      ],
    ),
  ).toEqual({ ids: [1], target: 0, missing: 2 });
});
