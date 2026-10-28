// @vitest-environment jsdom
import { afterEach, expect, it } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { MediaSnapshot } from "../../types";
import { TitleStrip } from "./title-strip";

afterEach(cleanup);

it("prefers season artwork and retains metadata after image failure", () => {
  const media = {
    type: "tv",
    tmdbId: 1399,
    title: "Game of Thrones",
    year: 2011,
    season: 1,
    posterPath: "/series.jpg",
    seasonDetails: {
      number: 1,
      name: "Season 1",
      episodeCount: 10,
      posterPath: "/season.jpg",
      overview: "Season overview",
    },
  } as MediaSnapshot;
  render(<TitleStrip media={media} />);
  const poster = screen.getByAltText(
    "Game of Thrones poster",
  ) as HTMLImageElement;
  expect(poster.src).toContain("/season.jpg");
  fireEvent.error(poster);
  expect(poster.hidden).toBe(true);
  expect(screen.getByText("Season overview")).toBeTruthy();
  expect(screen.getByText(/10 episodes/)).toBeTruthy();
});
