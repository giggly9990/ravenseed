// @vitest-environment jsdom
import { afterEach, expect, it } from "vitest";
import { act, cleanup, renderHook } from "@testing-library/react";
import { useSettingsDraft } from "./use-settings-draft";

afterEach(cleanup);

it("syncs a refetch after the user reverted their edit, but keeps real edits", () => {
  const { result, rerender } = renderHook(
    ({ saved }: { saved?: { url: string } }) =>
      useSettingsDraft(saved, { url: "" }),
    { initialProps: { saved: { url: "a" } } },
  );
  act(() => result.current[1]({ url: "edited" }));
  act(() => result.current[1]({ url: "a" }));
  expect(result.current[3].dirty).toBe(false);
  rerender({ saved: { url: "b" } });
  expect(result.current[0]).toEqual({ url: "b" });
  expect(result.current[3].changes).toBe(0);

  act(() => result.current[1]({ url: "mine" }));
  rerender({ saved: { url: "c" } });
  expect(result.current[0]).toEqual({ url: "mine" });
  expect(result.current[3].saved).toEqual({ url: "c" });
  expect(result.current[3].dirty).toBe(true);
});
