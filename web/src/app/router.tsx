import {
  createRootRoute,
  createRoute,
  createRouter,
  lazyRouteComponent,
} from "@tanstack/react-router";
import { resolutions } from "./shared";
import { Layout } from "./layout";
// The layout already imports the upload queue, so it stays in the main chunk.
import { UploadQueue } from "../components/upload-queue";

// Pages load as separate chunks; the router fetches a page's chunk before it
// renders the route, and on link hover or focus.
const SearchPage = lazyRouteComponent(
  () => import("../features/search/search-page"),
  "SearchPage",
);
const HistoryPage = lazyRouteComponent(
  () => import("../features/history/history-page"),
  "HistoryPage",
);
const SettingsPage = lazyRouteComponent(
  () => import("../features/settings/settings-page"),
  "SettingsPage",
);
const MonitorsPage = lazyRouteComponent(
  () => import("../features/monitors/monitors-page"),
  "MonitorsPage",
);
const UpgradesPage = lazyRouteComponent(
  () => import("../features/upgrades/upgrades-page"),
  "UpgradesPage",
);

const rootRoute = createRootRoute({ component: Layout });
export const searchRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  component: SearchPage,
  validateSearch: (
    search: Record<string, unknown>,
  ): { job?: string; resolution?: string } => ({
    job: typeof search.job === "string" ? search.job : undefined,
    resolution:
      typeof search.resolution === "string" &&
      search.resolution.split(",").every((value) => resolutions.includes(value))
        ? search.resolution
        : undefined,
  }),
});
const historyRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/history",
  component: HistoryPage,
});
const settingsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/settings",
  component: SettingsPage,
  validateSearch: (search: Record<string, unknown>): { section?: string } => ({
    section: typeof search.section === "string" ? search.section : undefined,
  }),
});
const uploadsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/uploads",
  component: UploadQueue,
});
const monitorsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/monitors",
  component: MonitorsPage,
});
const upgradesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/upgrades",
  component: UpgradesPage,
});
// Icon gallery for development only; production builds drop it.
const devIconsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/dev/icons",
  component: import.meta.env.DEV
    ? lazyRouteComponent(
        () => import("../features/dev/icons-page"),
        "DevIconsPage",
      )
    : () => null,
});
export const router = createRouter({
  defaultPreload: "intent",
  routeTree: rootRoute.addChildren([
    searchRoute,
    historyRoute,
    settingsRoute,
    uploadsRoute,
    monitorsRoute,
    upgradesRoute,
    ...(import.meta.env.DEV ? [devIconsRoute] : []),
  ]),
});
declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
