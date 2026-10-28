// @vitest-environment jsdom
import { useState } from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { URLField, URLPrivacyProvider, URLPrivacyToggle } from "./url-privacy";

afterEach(() => {
  cleanup();
  localStorage.clear();
  vi.restoreAllMocks();
});

function Fields({ save = () => {} }: { save?: (data: FormData) => void }) {
  const [qui, setQui] = useState("http://qui:7476");
  const [ua, setUA] = useState("http://ua:5000");
  return (
    <URLPrivacyProvider>
      <URLPrivacyToggle />
      <form
        onSubmit={(event) => {
          event.preventDefault();
          save(new FormData(event.currentTarget));
        }}
      >
        <URLField
          label="qui URL"
          name="qui"
          value={qui}
          required
          onChange={(event) => setQui(event.target.value)}
        />
        <URLField
          label="WebUI URL"
          name="ua"
          value={ua}
          onChange={(event) => setUA(event.target.value)}
        />
        <button type="submit">Save</button>
      </form>
    </URLPrivacyProvider>
  );
}

it("masks both URLs, temporarily reveals one and remembers the preference", () => {
  const view = render(<Fields />);
  const qui = screen.getByLabelText("qui URL") as HTMLInputElement;
  const ua = screen.getByLabelText("WebUI URL") as HTMLInputElement;
  expect(qui.type).toBe("url");
  expect(qui.dataset.urlField).toBe("true");
  expect(ua.dataset.urlField).toBe("true");
  const mask = screen.getByRole("checkbox", { name: "Mask URLs on screen" });
  expect((mask as HTMLInputElement).checked).toBe(false);
  fireEvent.click(screen.getByLabelText("Mask URLs on screen"));
  expect(qui.type).toBe("password");
  expect(ua.type).toBe("password");
  fireEvent.click(screen.getByRole("button", { name: "Show qui URL" }));
  expect(qui.type).toBe("url");
  expect(ua.type).toBe("password");
  fireEvent.blur(qui, { relatedTarget: screen.getByText("Save") });
  expect(qui.type).toBe("password");
  expect(localStorage.getItem("ravenseed.maskConnectionUrls")).toBe("true");
  expect(localStorage.length).toBe(1);
  view.unmount();
  render(<Fields />);
  expect((screen.getByLabelText("WebUI URL") as HTMLInputElement).type).toBe(
    "password",
  );
  fireEvent.click(screen.getByLabelText("Mask URLs on screen"));
  expect((screen.getByLabelText("qui URL") as HTMLInputElement).type).toBe(
    "url",
  );
});

it("preserves URL validation and submits actual values while masked", () => {
  const save = vi.fn();
  render(<Fields save={save} />);
  fireEvent.click(screen.getByLabelText("Mask URLs on screen"));
  const qui = screen.getByLabelText("qui URL") as HTMLInputElement;
  fireEvent.change(qui, { target: { value: "invalid-url" } });
  expect(qui.checkValidity()).toBe(false);
  fireEvent.change(qui, { target: { value: "http://new-qui:7476" } });
  expect(qui.checkValidity()).toBe(true);
  fireEvent.click(screen.getByText("Save"));
  expect(save).toHaveBeenCalledOnce();
  expect(save.mock.calls[0][0].get("qui")).toBe("http://new-qui:7476");
  expect(save.mock.calls[0][0].get("ua")).toBe("http://ua:5000");
});

it("still allows masking when browser storage is unavailable", () => {
  vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
    throw new Error("Blocked");
  });
  vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
    throw new Error("Blocked");
  });
  render(<Fields />);
  fireEvent.click(screen.getByLabelText("Mask URLs on screen"));
  expect((screen.getByLabelText("qui URL") as HTMLInputElement).type).toBe(
    "password",
  );
});

it("warns about plain http to a public host without blocking saving", () => {
  const save = vi.fn();
  render(<Fields save={save} />);
  const qui = screen.getByLabelText("qui URL") as HTMLInputElement;
  const warning = /API key is sent\s+unencrypted/;
  expect(screen.queryByText(warning)).toBeNull();
  fireEvent.change(qui, { target: { value: "http://qui.example.com:7476" } });
  const notice = screen.getByText(warning);
  expect(qui.getAttribute("aria-describedby")).toBe(notice.id);
  fireEvent.click(screen.getByText("Save"));
  expect(save).toHaveBeenCalledOnce();
  fireEvent.change(qui, { target: { value: "https://qui.example.com:7476" } });
  expect(screen.queryByText(warning)).toBeNull();
  expect(qui.hasAttribute("aria-describedby")).toBe(false);
});
