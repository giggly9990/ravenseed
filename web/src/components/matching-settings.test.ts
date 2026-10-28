import { describe, expect, it } from "vitest";
import {
  parseAliases,
  parseMappings,
  rulesFromDraft,
} from "./matching-settings";

describe("matching settings input", () => {
  it("normalizes aliases and rejects duplicate or malformed entries", () => {
    expect(parseAliases("old = New\nlegacy = NEW")).toEqual({
      OLD: "NEW",
      LEGACY: "NEW",
    });
    expect(() => parseAliases("OLD = NEW\nold = OTHER")).toThrow("Duplicate");
    expect(() => parseAliases("OLD NEW")).toThrow("one group alias");
  });
  it("requires a media identity for absolute episode mappings", () => {
    expect(
      parseMappings("tmdb:1399 13 = S02E01\nimdb:tt1234567 25 = S02E01"),
    ).toEqual([
      { tmdbId: 1399, absolute: 13, season: 2, episode: 1 },
      { imdbId: "tt1234567", absolute: 25, season: 2, episode: 1 },
    ]);
    expect(() => parseMappings("13 = S02E01")).toThrow("one episode mapping");
    expect(parseMappings("\n  \n")).toEqual([]);
  });
  it("builds the matching payload from editable rows, skipping blank rows", () => {
    expect(
      rulesFromDraft({
        rules: { matchAlternateTitles: false, sizeTolerancePercent: 0.2 },
        aliases: [
          { from: "old", to: "new" },
          { from: "", to: "" },
        ],
        ignored: ["MULTi", " "],
        mappings: [
          { id: "tmdb:1399", absolute: "13", season: "2", episode: "1" },
          { id: "", absolute: "", season: "", episode: "" },
        ],
      }),
    ).toEqual({
      matchAlternateTitles: false,
      sizeTolerancePercent: 0.2,
      groupAliases: { OLD: "NEW" },
      ignoredTags: ["MULTi"],
      episodeMappings: [{ tmdbId: 1399, absolute: 13, season: 2, episode: 1 }],
    });
    expect(() =>
      rulesFromDraft({
        rules: { matchAlternateTitles: true, sizeTolerancePercent: 0 },
        aliases: [],
        ignored: [],
        mappings: [{ id: "tmdb:1", absolute: "3", season: "", episode: "" }],
      }),
    ).toThrow("one episode mapping");
  });
});
