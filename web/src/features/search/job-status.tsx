import { useState, type ReactNode } from "react";
import { RefreshIcon } from "../../components/icons";
import type { Job } from "../../types";
import { Button } from "../../components/ui/button";
import { Dialog } from "../../components/ui/dialog";
import { SeedProgress, type SeedState } from "../../components/raven-reactions";
import { date } from "../../app/shared";
import { duration as elapsed } from "../../lib/time";

const seedState = (status: string): SeedState =>
  status === "completed" ? "done" : status === "failed" ? "failed" : "pending";

const methodLabel = (method: string) =>
  ({ imdb: "IMDb", tmdb: "TMDb", tvdb: "TVDb", title: "title" })[method] ??
  method;

/** Seeds for a search that has been sent but has no job yet. */
export function PendingSeeds({ names }: { names: string[] }) {
  return (
    <div className="disc-seeds">
      <SeedProgress
        items={names.map((label, i) => ({
          key: `${i}:${label}`,
          label,
          state: "pending",
        }))}
        summary={`0 of ${names.length} indexers`}
      />
    </div>
  );
}

export function JobStatus({
  job,
  pending,
  refreshing,
  refresh,
}: {
  job: Job;
  /** Replaces the status while a new search for this title starts. */
  pending?: ReactNode;
  refreshing: boolean;
  refresh: () => void;
}) {
  const [details, setDetails] = useState(false);
  const answered = job.indexers.filter((i) =>
    ["completed", "failed"].includes(i.status),
  ).length;
  const failed = job.indexers.filter((i) => i.status === "failed").length;
  const results = job.indexers.reduce((sum, i) => sum + i.resultCount, 0);
  const running = job.status === "running";
  const duration = elapsed(job.startedAt, job.completedAt);
  const tone =
    job.status === "failed"
      ? "offline"
      : running
        ? "warn"
        : failed
          ? "warn"
          : "online";
  if (pending) return pending;
  return (
    <div className="disc-status">
      {running ? (
        <div className="disc-seeds">
          <SeedProgress
            items={job.indexers.map((i) => ({
              key: i.indexerId,
              label: i.name,
              state: seedState(i.status),
            }))}
            summary={`${answered} of ${job.indexers.length} indexers · ${results} results`}
          />
        </div>
      ) : (
        <div className="disc-status-line">
          <span className={`status-dot ${tone}`} aria-hidden="true" />
          <span title={`Started ${date(job.startedAt)}`}>
            {job.status === "failed" ? "Search failed" : "Complete"}
            {" · "}
            <span className="num">
              {answered - failed}/{job.indexers.length}
            </span>{" "}
            indexers · <span className="num">{job.resultCount}</span> results
            {duration && (
              <>
                {" · "}
                <span className="num">{duration}</span>
              </>
            )}
            {job.cacheHit && " · cached"}
          </span>
          <Button
            className="sm quiet"
            aria-label="Refresh this search"
            title="Search every indexer again, skipping the cache"
            disabled={refreshing}
            onClick={refresh}
          >
            <RefreshIcon size={12} className={refreshing ? "spin" : ""} />
            Refresh
          </Button>
        </div>
      )}
      <ul className="disc-indexer-chips" aria-label="Indexers searched">
        {job.indexers.map((i) => {
          const strategy = i.strategy
            ? `${methodLabel(i.strategy.method)} search ${i.strategy.query}`
            : "";
          const state =
            i.status === "failed"
              ? (i.error ?? "Failed")
              : i.status === "completed"
                ? `${i.resultCount} results`
                : "Searching";
          return (
            <li
              key={i.indexerId}
              className={`disc-ichip ${i.status}`}
              title={[`${i.name}: ${state}`, strategy]
                .filter(Boolean)
                .join(" · ")}
            >
              {i.name}
              {i.status === "failed" ? (
                <span className="disc-ichip-n">
                  <span aria-hidden="true">
                    {/time/i.test(i.error ?? "") ? "timeout" : "error"}
                  </span>
                  <span className="visually-hidden">: {state}</span>
                </span>
              ) : i.status === "completed" ? (
                <span className="disc-ichip-n">
                  <span className="visually-hidden">: </span>
                  {i.resultCount}
                </span>
              ) : (
                <span className="disc-ichip-n">
                  <span aria-hidden="true">…</span>
                  <span className="visually-hidden">: searching</span>
                </span>
              )}
            </li>
          );
        })}
        <li>
          <button
            type="button"
            className="link-button disc-details-link"
            onClick={() => setDetails(true)}
          >
            Details
          </button>
        </li>
      </ul>
      <Dialog
        open={details}
        onOpenChange={setDetails}
        title="Search details"
        description={`Started ${date(job.startedAt)}${duration ? ` · took ${duration}` : ""}${job.cacheHit ? " · served from cache" : ""}`}
        wide
      >
        <div className="table-wrap">
          <table className="data-table compact">
            <thead>
              <tr>
                <th>Indexer</th>
                <th>Status</th>
                <th className="num">Results</th>
                <th>Search by</th>
                <th>Query</th>
                <th>Why</th>
              </tr>
            </thead>
            <tbody>
              {job.indexers.map((i) => (
                <tr key={i.indexerId}>
                  <td>{i.name}</td>
                  <td>
                    {i.status === "failed" ? (
                      <span className="pill danger" title={i.error}>
                        {i.error ?? "Failed"}
                      </span>
                    ) : (
                      <span
                        className={`pill ${i.status === "completed" ? "ok" : "quiet"}`}
                      >
                        {i.status}
                      </span>
                    )}
                  </td>
                  <td className="num">{i.resultCount}</td>
                  <td>{i.strategy ? methodLabel(i.strategy.method) : "—"}</td>
                  <td className="mono small break">
                    {i.strategy?.query ?? "—"}
                  </td>
                  <td className="small muted">
                    {i.strategy?.reason ?? "Default"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        {job.matchingRules && (
          <details className="disc-rules">
            <summary>Matching rules used for this search</summary>
            <pre>{JSON.stringify(job.matchingRules, null, 2)}</pre>
          </details>
        )}
      </Dialog>
    </div>
  );
}
