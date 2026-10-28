// @vitest-environment jsdom
import { afterEach, expect, it } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { useState } from "react";
import { Select } from "./select";
import { openPicker, closePicker, pick } from "../../test/pickers";

afterEach(cleanup);

function Pickers() {
  const [values, setValues] = useState<string[]>([]);
  const [single, setSingle] = useState("a");
  return (
    <>
      <button>Outside</button>
      <Select
        aria-label="Indexers"
        multiple
        value={values}
        onChange={(e) =>
          setValues(e.target.selectedOptions.map((o) => o.value))
        }
      >
        <option value="a">Atlas</option>
        <option value="b" disabled>
          Disabled tracker
        </option>
      </Select>
      <Select
        aria-label="Category"
        value={single}
        onChange={(e) => setSingle(e.target.value)}
      >
        <option value="a">Movies</option>
        <option value="b">TV</option>
      </Select>
    </>
  );
}

it("keeps multiple choices open and dismisses on outside interaction", async () => {
  render(<Pickers />);
  await openPicker("Indexers");
  fireEvent.click(screen.getByRole("menuitemcheckbox", { name: "Atlas" }));
  expect(
    screen
      .getByRole("menuitemcheckbox", { name: "Atlas" })
      .getAttribute("aria-checked"),
  ).toBe("true");
  expect(
    screen
      .getByRole("menuitemcheckbox", { name: "Disabled tracker" })
      .getAttribute("aria-disabled"),
  ).toBe("true");
  await new Promise((resolve) => setTimeout(resolve, 0));
  fireEvent.pointerDown(screen.getByRole("button", { name: "Outside" }), {
    pointerType: "mouse",
  });
  await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
});

it("dismisses with Escape and closes a single picker after selection", async () => {
  render(<Pickers />);
  await openPicker("Indexers");
  closePicker();
  expect(screen.queryByRole("menu")).toBeNull();
  await pick("Category", "TV");
  expect(screen.queryByRole("menu")).toBeNull();
  expect(
    screen.getByRole("button", { name: "Category" }).textContent,
  ).toContain("TV");
});

it("opens from a click on its wrapping label and stays shut inside a disabled fieldset", async () => {
  render(
    <>
      <label>
        Default target
        <Select aria-label="Default target" value="a" onChange={() => {}}>
          <option value="a">Atlas</option>
        </Select>
      </label>
      <fieldset disabled>
        <Select aria-label="Locked" value="a" onChange={() => {}}>
          <option value="a">Atlas</option>
        </Select>
      </fieldset>
    </>,
  );
  fireEvent.click(screen.getByText("Default target"));
  expect(await screen.findByRole("menu")).toBeTruthy();
  fireEvent.keyDown(screen.getByRole("menu"), { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
  fireEvent.keyDown(screen.getByRole("button", { name: "Locked" }), {
    key: "ArrowDown",
  });
  expect(screen.queryByRole("menu")).toBeNull();
});
