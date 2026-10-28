import { afterEach, expect, it, vi } from "vitest";
import { newPresetId } from "./upload-presets";

afterEach(() => {
  vi.unstubAllGlobals();
});

it("creates preset IDs without crypto.randomUUID (plain-http LAN origins)", () => {
  vi.stubGlobal("crypto", {
    getRandomValues: (bytes: Uint8Array) => bytes.fill(171),
  });
  expect(newPresetId()).toBe("ab".repeat(12));
  expect(newPresetId()).toMatch(/^[a-zA-Z0-9_-]{1,80}$/);
});

it("falls back to Math.random and skips IDs already in use", () => {
  vi.stubGlobal("crypto", undefined);
  const random = vi
    .spyOn(Math, "random")
    .mockReturnValueOnce(0)
    .mockReturnValue(0.5);
  const taken = "00".repeat(12);
  // The first 12 draws produce the taken ID; the next ID must differ.
  for (let i = 0; i < 11; i++) random.mockReturnValueOnce(0);
  expect(newPresetId([taken])).toBe("80".repeat(12));
  random.mockRestore();
});
