import { describe, expect, it } from "vitest";
import { shortFlag, trackerFlags } from "./tracker-flags";

describe("tracker flags", () => {
  it("shows reported freeleech and upload credit", () => {
    expect(
      trackerFlags({
        raw: { download_volume_factor: 0, upload_volume_factor: 2 },
      }),
    ).toEqual(["Freeleech", "2× upload"]);
    expect(
      trackerFlags({
        raw: { download_volume_factor: 0.5, upload_volume_factor: 0 },
      }),
    ).toEqual(["50% download", "No upload credit"]);
  });
  it("does not turn missing or invalid factors into freeleech", () => {
    expect(
      trackerFlags({ raw: {}, downloadFactor: 0, uploadFactor: 0 }),
    ).toEqual([]);
    expect(
      trackerFlags({
        raw: { download_volume_factor: null, upload_volume_factor: -1 },
      }),
    ).toEqual([]);
    expect(
      trackerFlags({
        raw: { download_volume_factor: 1, upload_volume_factor: 1 },
      }),
    ).toEqual([]);
  });
  it("shortens flags for table chips", () => {
    expect(
      [
        "Freeleech",
        "50% download",
        "2× upload",
        "No upload credit",
        "Other",
      ].map(shortFlag),
    ).toEqual(["FL", "50%", "2×↑", "0×↑", "Other"]);
  });
});
