// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import {
  iconRegistry,
  CloseIcon,
  RavenMarkIcon,
  SearchIcon,
  SpinnerIcon,
} from "./icons";

afterEach(cleanup);

describe("RavenSeed icons", () => {
  it("renders decorative icons hidden from assistive technology by default", () => {
    const { container } = render(<SearchIcon size={16} />);
    const icon = container.querySelector("svg");
    expect(icon?.getAttribute("width")).toBe("16");
    expect(icon?.getAttribute("height")).toBe("16");
    expect(icon?.getAttribute("aria-hidden")).toBe("true");
  });

  it("becomes accessible when labelled", () => {
    render(<RavenMarkIcon title="RavenSeed" />);
    expect(screen.getByRole("img", { name: "RavenSeed" })).toBeTruthy();
  });

  it("keeps the spinner compatible with the existing spin class", () => {
    const { container } = render(<SpinnerIcon className="spin" />);
    expect(container.querySelector("svg")?.classList.contains("spin")).toBe(
      true,
    );
  });

  it("exposes the frozen semantic registry", () => {
    expect(iconRegistry["close"]).toBe(CloseIcon);
    expect(iconRegistry["candidate"]).toBeDefined();
    expect(iconRegistry["target"]).toBeDefined();
    expect(iconRegistry["release-family"]).toBeDefined();
    expect(iconRegistry["raven-mark"]).toBe(RavenMarkIcon);
  });
});
