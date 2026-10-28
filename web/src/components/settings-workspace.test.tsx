// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { SaveBar, SettingsWorkspace } from "./settings-workspace";
import { useSettingsDraft } from "../hooks/use-settings-draft";
import { pick } from "../test/pickers";

afterEach(cleanup);

function Draft({ save }: { save: (value: string) => void }) {
  const [value, setValue, accept, draft] = useSettingsDraft("", "");
  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        save(value);
        accept(value);
      }}
    >
      <label>
        Connection URL
        <input
          value={value}
          onChange={(event) => setValue(event.target.value)}
        />
      </label>
      <label>
        API key
        <input type="password" />
      </label>
      <SaveBar changes={draft.changes} onDiscard={draft.reset} />
    </form>
  );
}

function setup(onSectionChange?: (id: string) => void, section?: "uploads") {
  const save = vi.fn();
  render(
    <SettingsWorkspace
      section={section}
      onSectionChange={onSectionChange}
      panels={{
        qui: <Draft save={save} />,
        uploads: <h2>Upload-Assistant</h2>,
        arr: <h2>Sonarr &amp; Radarr</h2>,
        metadata: <h2>TMDb metadata</h2>,
        defaults: <h2>Defaults</h2>,
        indexerSearch: <h2>Per-indexer search</h2>,
        matching: <h2>Matching rules</h2>,
        policies: <h2>Indexer policies</h2>,
      }}
    />,
  );
  return save;
}

it("shows one section and retains unsaved edits when switching sections", () => {
  const save = setup();
  fireEvent.change(screen.getByRole("textbox", { name: "Connection URL" }), {
    target: { value: "http://qui:7476" },
  });
  fireEvent.click(screen.getByRole("tab", { name: /Upload-Assistant/ }));
  expect(screen.getAllByRole("tabpanel")).toHaveLength(1);
  expect(
    screen.getByRole("heading", { name: "Upload-Assistant" }),
  ).toBeTruthy();
  expect(screen.queryByRole("textbox")).toBeNull();
  fireEvent.click(screen.getByRole("tab", { name: /^qui/ }));
  expect((screen.getByRole("textbox") as HTMLInputElement).value).toBe(
    "http://qui:7476",
  );
  expect(save).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  expect(save).toHaveBeenCalledWith("http://qui:7476");
});

it("groups sections with short labels and no descriptions", () => {
  setup();
  expect(
    screen.getByRole("heading", { level: 1, name: "Settings" }),
  ).toBeTruthy();
  expect(screen.getAllByRole("tab").map((tab) => tab.textContent)).toEqual([
    "qui",
    "Upload-Assistant",
    "Sonarr & Radarr",
    "TMDb metadata",
    "Defaults",
    "Per-indexer search",
    "Matching rules",
    "Indexer policies",
  ]);
  expect(screen.getByText("Connections")).toBeTruthy();
  expect(screen.getByText("Searching")).toBeTruthy();
  expect(screen.getByText("Uploading")).toBeTruthy();
  expect(screen.getByLabelText("Mask URLs on screen")).toBeTruthy();
});

it("marks a dirty section in the nav, shows the save bar and discards", () => {
  setup();
  expect(screen.queryByRole("region", { name: "Unsaved changes" })).toBeNull();
  const input = screen.getByRole("textbox", {
    name: "Connection URL",
  }) as HTMLInputElement;
  fireEvent.change(input, { target: { value: "http://qui:7476" } });
  const bar = screen.getByRole("region", { name: "Unsaved changes" });
  expect(bar.textContent).toContain("1 unsaved change in qui");
  expect(
    screen.getByRole("tab", { name: "qui, unsaved changes" }),
  ).toBeTruthy();
  expect(screen.getByRole("tab", { name: "Upload-Assistant" })).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Discard" }));
  expect(input.value).toBe("");
  expect(screen.queryByRole("region", { name: "Unsaved changes" })).toBeNull();
  expect(screen.getByRole("tab", { name: "qui" })).toBeTruthy();
  // Editing back to the saved value is not a change either.
  fireEvent.change(input, { target: { value: "x" } });
  fireEvent.change(input, { target: { value: "" } });
  expect(screen.queryByRole("region", { name: "Unsaved changes" })).toBeNull();
});

it("supports keyboard navigation and the compact section picker", async () => {
  const change = vi.fn();
  setup(change);
  const qui = screen.getByRole("tab", { name: "qui" });
  qui.focus();
  fireEvent.keyDown(qui, { key: "ArrowDown" });
  const uploads = screen.getByRole("tab", { name: "Upload-Assistant" });
  expect(document.activeElement).toBe(uploads);
  expect(uploads.getAttribute("aria-selected")).toBe("true");
  expect(change).toHaveBeenLastCalledWith("uploads");
  fireEvent.keyDown(uploads, { key: "End" });
  expect(
    screen.getByRole("heading", { name: "Indexer policies" }),
  ).toBeTruthy();
  fireEvent.keyDown(document.activeElement!, { key: "ArrowDown" });
  expect(document.activeElement).toBe(qui);
  await pick("Settings section", "TMDb metadata");
  expect(screen.getAllByRole("tabpanel")).toHaveLength(1);
  expect(screen.getByRole("heading", { name: "TMDb metadata" })).toBeTruthy();
  expect(
    screen
      .getByRole("tab", { name: /TMDb metadata/ })
      .getAttribute("aria-selected"),
  ).toBe("true");
  expect(change).toHaveBeenLastCalledWith("metadata");
});

it("opens the section it is given", () => {
  setup(undefined, "uploads");
  expect(
    screen.getByRole("heading", { name: "Upload-Assistant" }),
  ).toBeTruthy();
});

it("no longer renders its own raven peek", () => {
  setup();
  fireEvent.focus(screen.getByLabelText("API key"));
  expect(screen.queryByText("👀")).toBeNull();
});

it("mounts a section's panel only once it is opened, then keeps it", () => {
  setup();
  expect(
    screen.queryByRole("heading", { name: "Matching rules", hidden: true }),
  ).toBeNull();
  fireEvent.click(screen.getByRole("tab", { name: /Matching/ }));
  fireEvent.click(screen.getByRole("tab", { name: /^qui/ }));
  expect(
    screen.getByRole("heading", { name: "Matching rules", hidden: true }),
  ).toBeTruthy();
});
