// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { ChipInput } from "./settings-workspace";
import { encodeScope } from "./indexer-policies";

afterEach(cleanup);

it("normalises encode scopes and rejects what the server would refuse", () => {
  expect(encodeScope("1080P:avc")).toBe("1080p:AVC");
  expect(encodeScope(" 2160p : hevc ")).toBe("2160p:HEVC");
  expect(encodeScope("1080I:avc")).toBe("1080i:AVC");
  expect(encodeScope("999p:AVC")).toBe("");
  expect(encodeScope("1080p")).toBe("");
  expect(encodeScope("1080p:AVC:x")).toBe("");
});

it("keeps rejected chip entries in the box instead of dropping them", () => {
  const onChange = vi.fn();
  render(
    <ChipInput
      label="Add encode size scope"
      values={[]}
      onChange={onChange}
      transform={encodeScope}
      invalidHint="Use resolution:codec"
    />,
  );
  const input = screen.getByLabelText("Add encode size scope");
  fireEvent.change(input, { target: { value: "1080P:avc, 9999p:AVC" } });
  fireEvent.keyDown(input, { key: "Enter" });
  expect(onChange).toHaveBeenCalledWith(["1080p:AVC"]);
  expect((input as HTMLInputElement).value).toBe("9999p:AVC");
  expect(input.getAttribute("aria-invalid")).toBe("true");
  expect(screen.getByRole("status").textContent).toBe("Use resolution:codec");
});
