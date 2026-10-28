import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, Outlet } from "@tanstack/react-router";
import {
  ExternalLinkIcon,
  MonitorIcon,
  RavenMarkIcon,
  SearchHistoryIcon,
  SearchIcon,
  SettingsIcon,
  UpgradeIcon,
  UploadIcon,
} from "../components/icons";
import { api } from "../api";
import type { Monitor, Page, Settings, UploadJobsPage } from "../types";
import { Button } from "../components/ui/button";
import { RavenPeek } from "../components/raven-reactions";
import {
  uploadableClasses,
  uploadableCount,
  useCandidateCounts,
} from "../hooks/use-candidate-counts";
import { activeUploadStatuses } from "../components/upload-queue";
import { ErrorNotice } from "./shared";

const badge = (n: number) => (n > 99 ? "99+" : String(n));

/** Sidebar count; screen readers get the sentence, not the abbreviated digits. */
function CountBadge({
  count,
  label,
  accent = false,
}: {
  count: number;
  label: string;
  accent?: boolean;
}) {
  if (count <= 0) return null;
  return (
    <span className={`count-badge${accent ? " accent" : ""}`}>
      <span aria-hidden="true">{badge(count)}</span>
      <span className="visually-hidden">
        {count} {label}
      </span>
    </span>
  );
}

export function Layout() {
  const [expired, setExpired] = useState(false);
  const session = useQuery({
    queryKey: ["browser-session"],
    queryFn: () => api<{ authenticated: boolean }>("/session"),
  });
  const settings = useQuery({
    queryKey: ["settings"],
    queryFn: () => api<Settings>("/settings"),
    enabled: !expired,
  });
  const ready = !expired && !!settings.data;
  const health = useQuery({
    queryKey: ["readiness"],
    queryFn: () => api<{ qui: string }>("/ready"),
    enabled: ready,
    refetchInterval: 30000,
  });
  const monitors = useQuery({
    queryKey: ["monitors"],
    queryFn: () => api<Monitor[]>("/monitors"),
    enabled: ready,
    refetchInterval: 30000,
  });
  const uploads = useQuery({
    queryKey: ["upload-queue", "sidebar"],
    // The status filter's total is the exact number of active jobs.
    queryFn: () =>
      api<UploadJobsPage>(
        `/uploads?limit=1&offset=0&status=${activeUploadStatuses.join(",")}`,
      ),
    enabled: ready,
    refetchInterval: 15000,
  });
  const upgrades = useQuery({
    queryKey: ["upgrades", "sidebar"],
    queryFn: () =>
      api<Page<unknown>>("/upgrades?level=upgrade&limit=1&offset=0"),
    enabled: ready,
    refetchInterval: 60000,
  });
  const upgradeBadge = upgrades.data?.total ?? 0;
  const enabledMonitors = monitors.data?.filter((m) => m.enabled);
  // One request counts every monitor; the Monitors page polls it faster.
  const counts = useCandidateCounts(enabledMonitors, uploadableClasses, 60_000);
  const monitorBadge = Object.values(counts).reduce(
    (sum, row) => sum + uploadableCount(row),
    0,
  );
  const activeUploads = uploads.data?.total ?? 0;
  const quiStatus = health.isError ? "unavailable" : health.data?.qui;
  useEffect(() => {
    const handler = () => {
      setExpired(true);
      window.location.assign(
        "/login?next=" +
          encodeURIComponent(window.location.pathname + window.location.search),
      );
    };
    window.addEventListener("auth-required", handler);
    return () => window.removeEventListener("auth-required", handler);
  }, []);
  if (expired) return null;
  return (
    <div className="app-shell">
      <aside className="sidebar">
        <Link to="/" className="brand" aria-label="RavenSeed home">
          <span className="brand-icon">
            <RavenMarkIcon size={20} color="currentColor" />
          </span>
          <span>RavenSeed</span>
        </Link>
        <nav aria-label="Main">
          <Link to="/" activeOptions={{ exact: true }}>
            <SearchIcon size={16} />
            <span>Discover</span>
          </Link>
          <Link to="/history">
            <SearchHistoryIcon size={16} />
            <span>History</span>
          </Link>
          <Link to="/monitors">
            <MonitorIcon size={16} />
            <span>Monitors</span>
            <CountBadge accent count={monitorBadge} label="upload candidates" />
          </Link>
          <Link to="/upgrades">
            <UpgradeIcon size={16} />
            <span>Upgrades</span>
            <CountBadge count={upgradeBadge} label="upgrades" />
          </Link>
          <Link to="/uploads">
            <UploadIcon size={16} />
            <span>Uploads</span>
            <CountBadge count={activeUploads} label="active uploads" />
          </Link>
          <Link to="/settings">
            <SettingsIcon size={16} />
            <span>Settings</span>
          </Link>
        </nav>
        <div className="sidebar-foot">
          <div className="connection" title={`qui: ${quiStatus ?? "checking"}`}>
            <span
              className={`status-dot ${quiStatus === "connected" ? "online" : quiStatus === "unavailable" ? "offline" : ""}`}
            />
            <span>
              {quiStatus === "connected"
                ? "qui connected"
                : quiStatus === "not_configured"
                  ? "qui not configured"
                  : quiStatus === "unavailable"
                    ? "qui unavailable"
                    : "Checking qui…"}
            </span>
          </div>
          <a href="/api/docs" target="_blank" rel="noreferrer">
            API docs <ExternalLinkIcon size={12} />
          </a>
          {session.data?.authenticated && (
            <form method="post" action="/logout">
              <Button type="submit" className="sm quiet">
                Sign out
              </Button>
            </form>
          )}
          <span className="version">RavenSeed v0.6.0</span>
        </div>
      </aside>
      <div className="workspace">
        <main>
          <ErrorNotice error={settings.error} />
          <Outlet />
        </main>
      </div>
      <RavenPeek />
    </div>
  );
}
