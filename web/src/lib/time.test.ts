import { expect, it } from "vitest";
import { duration, relativeTime, shortRelativeTime } from "./time";

const now = Date.parse("2026-10-24T12:00:00Z");
const ago = (ms: number) => new Date(now - ms).toISOString();

it("formats long and short relative times", () => {
  expect(relativeTime(ago(2_000), now)).toBe("just now");
  expect(relativeTime(ago(3 * 3_600_000), now)).toBe("3 hours ago");
  expect(shortRelativeTime(ago(20_000), now)).toBe("just now");
  expect(shortRelativeTime(ago(4 * 60_000), now)).toBe("4 min ago");
  expect(shortRelativeTime(ago(-3 * 3_600_000), now)).toBe("in 3 h");
  expect(shortRelativeTime(undefined, now)).toBe("");
});

it("formats durations and rejects missing or negative spans", () => {
  expect(duration(ago(4_200), new Date(now).toISOString())).toBe("4.2 s");
  expect(duration(ago(72_000), new Date(now).toISOString())).toBe("1 min 12 s");
  expect(duration(ago(0))).toBe("");
  expect(duration(new Date(now).toISOString(), ago(1_000))).toBe("");
});
