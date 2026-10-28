// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { parseMediaInput, TMDbSettings } from "./media-metadata";
import { api } from "../api";

vi.mock("../api", () => ({ api: vi.fn() }));
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});

it("recognizes IMDb and TMDb input forms without guessing invalid IDs", () => {
  for (const value of ["tt0133093", "imdb:tt0133093", " IMDB:tt0133093 "])
    expect(parseMediaInput(value)).toEqual({ imdbId: "tt0133093" });
  for (const value of ["603", "tmdb:603"])
    expect(parseMediaInput(value)).toEqual({ tmdbId: 603 });
  for (const value of ["imdb:603", "tmdb:tt0133093", "0", "-1", "2147483648"])
    expect(parseMediaInput(value)).toBeNull();
});

it("saves TMDb settings independently and clears the token input", async () => {
  vi.mocked(api).mockResolvedValue({
    metadata: { hasToken: true, language: "fr", environmentManaged: false },
  });
  render(
    <QueryClientProvider client={new QueryClient()}>
      <TMDbSettings />
    </QueryClientProvider>,
  );
  const token = screen.getByLabelText(
    "TMDb API key or Read Access Token",
  ) as HTMLInputElement;
  expect(token.type).toBe("password");
  expect(token.closest(".secret-input")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Save" })).toBeNull();
  fireEvent.change(token, { target: { value: "private-token" } });
  fireEvent.change(screen.getByLabelText("Metadata language"), {
    target: { value: "fr" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  await screen.findByText("TMDb settings saved.");
  expect(api).toHaveBeenCalledWith("/settings/metadata", {
    method: "PUT",
    body: JSON.stringify({
      token: "private-token",
      language: "fr",
      clearToken: false,
    }),
  });
  expect(
    (
      screen.getByLabelText(
        "TMDb API key or Read Access Token",
      ) as HTMLInputElement
    ).value,
  ).toBe("");
});
