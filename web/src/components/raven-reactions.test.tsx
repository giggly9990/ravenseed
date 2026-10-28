// @vitest-environment jsdom
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import {
  RavenError,
  RavenPeek,
  SeedProgress,
  resetRavenErrors,
  useRavenReaction,
} from "./raven-reactions";
import { ErrorNotice } from "../app/shared";

afterEach(() => {
  cleanup();
  resetRavenErrors();
  vi.useRealTimers();
});

function Reactions({ onShow }: { onShow?: () => void }) {
  const raven = useRavenReaction();
  return (
    <>
      {raven.reaction}
      <button
        onClick={() =>
          raven.show("opportunities", { count: 2, target: "Orbit", onShow })
        }
      >
        Opportunities
      </button>
      <button onClick={raven.hide}>Stop</button>
    </>
  );
}

it("shows got coke? with the count, then leaves and cancels timers", () => {
  vi.useFakeTimers();
  const view = render(<Reactions />);
  fireEvent.click(screen.getByText("Opportunities"));
  expect(screen.getByText("got coke?")).toBeTruthy();
  expect(screen.getByRole("status").textContent).toContain(
    "2 opportunities on Orbit",
  );
  act(() => vi.advanceTimersByTime(6000));
  expect(screen.queryByText("got coke?")).toBeNull();
  fireEvent.click(screen.getByText("Opportunities"));
  fireEvent.click(screen.getByText("Stop"));
  expect(screen.queryByText("got coke?")).toBeNull();
  fireEvent.click(screen.getByText("Opportunities"));
  view.unmount();
  expect(vi.getTimerCount()).toBe(0);
});

it("lets the bubble jump to the opportunities", () => {
  const onShow = vi.fn();
  render(<Reactions onShow={onShow} />);
  fireEvent.click(screen.getByText("Opportunities"));
  fireEvent.click(screen.getByRole("button", { name: "show them" }));
  expect(onShow).toHaveBeenCalledOnce();
  expect(screen.queryByText("got coke?")).toBeNull();
});

it("keeps error details accessible alongside the decorative raven", () => {
  render(<RavenError>Could not connect to qui</RavenError>);
  expect(screen.getByRole("alert").textContent).toContain(
    "Could not connect to qui",
  );
  expect(screen.getByText("xD").closest('[aria-hidden="true"]')).toBeTruthy();
});

it("escalates when the same error comes back", () => {
  const first = render(<RavenError>qui timed out</RavenError>);
  expect(screen.getByText("xD")).toBeTruthy();
  first.unmount();
  const second = render(<RavenError>qui timed out</RavenError>);
  expect(screen.getByText("xDD")).toBeTruthy();
  second.unmount();
  render(<RavenError>qui timed out</RavenError>);
  expect(screen.getByText("xDDD")).toBeTruthy();
  cleanup();
  render(<RavenError>Something else</RavenError>);
  expect(screen.getByText("xD")).toBeTruthy();
});

it("escalates when a fresh error with the same message replaces the old one", () => {
  const view = render(<ErrorNotice error={new Error("qui returned 502")} />);
  expect(screen.getByText("xD")).toBeTruthy();
  view.rerender(<ErrorNotice error={new Error("qui returned 502")} />);
  expect(screen.getByText("xDD")).toBeTruthy();
  const same = new Error("qui returned 502");
  view.rerender(<ErrorNotice error={same} />);
  view.rerender(<ErrorNotice error={same} />);
  expect(screen.getByText("xDDD")).toBeTruthy();
});

it("perches on a focused secret field and covers its eyes when revealed", () => {
  render(
    <>
      <RavenPeek />
      <input aria-label="URL" type="password" data-url-field="" />
      <input aria-label="Token" type="password" />
      <input aria-label="Name" />
    </>,
  );
  const token = screen.getByLabelText("Token") as HTMLInputElement;
  expect(screen.queryByTestId("raven-peek")).toBeNull();
  act(() => token.focus());
  expect(screen.getByTestId("raven-peek")).toBeTruthy();
  act(() => (screen.getByLabelText("Name") as HTMLInputElement).focus());
  expect(screen.queryByTestId("raven-peek")).toBeNull();
  act(() => (screen.getByLabelText("URL") as HTMLInputElement).focus());
  expect(screen.queryByTestId("raven-peek")).toBeNull();
  act(() => token.focus());
  act(() => {
    token.type = "text";
    token.dispatchEvent(new Event("input", { bubbles: true }));
  });
  expect(screen.getByTestId("raven-peek").className).toContain("covered");
});

it("drops a seed per indexer and reports progress", () => {
  render(
    <SeedProgress
      items={[
        { key: 1, label: "Atlas", state: "done" },
        { key: 2, label: "Orbit", state: "failed" },
        { key: 3, label: "Beacon", state: "pending" },
      ]}
      summary="2 of 3 indexers"
    />,
  );
  const bar = screen.getByRole("progressbar", { name: "Indexers answered" });
  expect(bar.getAttribute("aria-valuenow")).toBe("2");
  expect(screen.getByText("spread the seed")).toBeTruthy();
  expect(screen.getByTitle("Orbit: failed")).toBeTruthy();
});

it("keeps got coke? up while the bubble is hovered or focused", () => {
  vi.useFakeTimers();
  render(<Reactions onShow={() => {}} />);
  fireEvent.click(screen.getByText("Opportunities"));
  const bubble = screen.getByText("got coke?").parentElement!;
  fireEvent.mouseEnter(bubble);
  act(() => vi.advanceTimersByTime(20_000));
  expect(screen.getByText("got coke?")).toBeTruthy();
  fireEvent.mouseLeave(bubble);
  act(() => vi.advanceTimersByTime(6000));
  expect(screen.queryByText("got coke?")).toBeNull();
  expect(screen.getByRole("status").textContent).toBe("");
});
