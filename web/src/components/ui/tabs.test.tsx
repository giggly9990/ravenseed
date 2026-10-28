// @vitest-environment jsdom
import { afterEach, expect, it } from "vitest";
import { useState } from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { onTabKeyDown } from "./tabs";

afterEach(cleanup);

const ids = ["a", "b", "c"] as const;
function Tabs() {
  const [tab, setTab] = useState<(typeof ids)[number]>("a");
  return (
    <div role="tablist">
      {ids.map((id) => (
        <button
          key={id}
          role="tab"
          aria-selected={tab === id}
          tabIndex={tab === id ? 0 : -1}
          onClick={() => setTab(id)}
          onKeyDown={(e) => onTabKeyDown(e, ids, tab, setTab)}
        >
          {id}
        </button>
      ))}
    </div>
  );
}

it("moves selection and focus with arrow keys, wrapping at the ends", () => {
  render(<Tabs />);
  const tab = (name: string) => screen.getByRole("tab", { name });
  fireEvent.keyDown(tab("a"), { key: "ArrowLeft" });
  expect(tab("c").getAttribute("aria-selected")).toBe("true");
  expect(document.activeElement).toBe(tab("c"));
  fireEvent.keyDown(tab("c"), { key: "ArrowRight" });
  expect(document.activeElement).toBe(tab("a"));
  fireEvent.keyDown(tab("a"), { key: "End" });
  expect(tab("c").tabIndex).toBe(0);
  expect(tab("a").tabIndex).toBe(-1);
});
