import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ArrowRightIcon, SpinnerIcon } from "../../components/icons";
import { api } from "../../api";
import type { Job, Page } from "../../types";
import { Pager } from "../../components/ui/pager";
import { ErrorNotice, StatusIcon } from "../../app/shared";
import { RavenReaction } from "../../components/raven-reactions";
import { duration, relativeTime } from "../../lib/time";
import { useNow } from "../../hooks/use-now";

const PAGE = 25;

const statusTone: Record<string, string> = {
  completed: "ok",
  failed: "danger",
  running: "info",
};

function describe(j: Job) {
  const media = j.request.media;
  const identifier =
    media.imdbId ||
    (media.tmdbId
      ? `TMDb ${media.tmdbId}`
      : media.tvdbId
        ? `TVDb ${media.tvdbId}`
        : "Unknown title");
  const title =
    j.media?.title?.trim() ||
    media.title?.trim() ||
    j.media?.originalTitle?.trim() ||
    media.originalTitle?.trim() ||
    identifier;
  const year = j.media?.year || media.year;
  const scope = [
    media.type.toUpperCase(),
    media.season !== undefined ? `Season ${media.season}` : "",
    media.episode !== undefined ? `Episode ${media.episode}` : "",
  ]
    .filter(Boolean)
    .join(" · ");
  return { identifier, title, year, scope };
}

export function HistoryPage() {
  const [offset, setOffset] = useState(0);
  const history = useQuery({
    queryKey: ["history", offset],
    queryFn: () => api<Page<Job>>(`/searches?limit=${PAGE}&offset=${offset}`),
    // Poll while a search runs; otherwise only refresh occasionally for
    // searches started elsewhere.
    refetchInterval: (query) =>
      query.state.data?.items.some((j) => j.status === "running")
        ? 5000
        : 60000,
  });
  // Re-renders so "started 2 minutes ago" keeps moving between refetches.
  useNow();
  const total = history.data?.total ?? 0;
  return (
    <>
      <header className="page-header">
        <h1>History</h1>
        {history.data && (
          <span className="count-badge" aria-label={`${total} saved searches`}>
            {total}
          </span>
        )}
        <span className="hint">Searches and their evidence, kept locally.</span>
      </header>
      <ErrorNotice error={history.error} />
      {history.isLoading ? (
        <p role="status" className="muted hist-loading">
          <SpinnerIcon size={16} className="spin" /> Loading history
        </p>
      ) : history.data?.items.length ? (
        <section className="panel table-wrap">
          <table className="data-table hist-table">
            <thead>
              <tr>
                <th scope="col">Title</th>
                <th scope="col">Status</th>
                <th scope="col">Indexers</th>
                <th scope="col" className="num">
                  Results
                </th>
                <th scope="col" className="num">
                  Families
                </th>
                <th scope="col" className="num">
                  Opportunities
                </th>
                <th scope="col">Started</th>
                <th scope="col">Duration</th>
                <th scope="col">
                  <span className="visually-hidden">Open</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {history.data.items.map((j) => {
                const { identifier, title, year, scope } = describe(j);
                const target = j.indexers.find(
                  (i) => i.indexerId === j.request.targetIndexerId,
                )?.name;
                const failedIndexers = j.indexers.filter(
                  (i) => i.status === "failed" || i.error,
                ).length;
                return (
                  <tr key={j.id}>
                    <td className="hist-title">
                      <strong>
                        {title}
                        {year ? (
                          <span className="faint hist-year"> {year}</span>
                        ) : null}
                      </strong>
                      <span className="cell-sub">
                        {scope}
                        {title !== identifier && (
                          <span className="mono"> · {identifier}</span>
                        )}
                      </span>
                    </td>
                    <td>
                      <span
                        className={`pill hist-status ${statusTone[j.status] ?? "quiet"}`}
                      >
                        <StatusIcon status={j.status} />
                        {j.status}
                      </span>
                    </td>
                    <td className="hist-indexers">
                      <span>{j.indexers.length} indexers</span>
                      <span className="cell-sub">
                        {target ? `Target ${target}` : "No target"}
                        {failedIndexers > 0 && (
                          <span className="hist-warn">
                            {" "}
                            · {failedIndexers} failed
                          </span>
                        )}
                      </span>
                    </td>
                    <td className="num mono">{j.resultCount}</td>
                    <td className="num mono">{j.familyCount}</td>
                    <td className="num mono">
                      {j.candidateCount > 0 ? (
                        <span className="hist-hot">{j.candidateCount}</span>
                      ) : (
                        <span className="faint">0</span>
                      )}
                    </td>
                    <td className="small muted">
                      <time
                        dateTime={j.startedAt}
                        title={new Date(j.startedAt).toLocaleString()}
                      >
                        {relativeTime(j.startedAt)}
                      </time>
                    </td>
                    <td className="small muted mono">
                      {j.completedAt
                        ? duration(j.startedAt, j.completedAt)
                        : "In progress"}
                    </td>
                    <td className="actions">
                      <Link
                        to="/"
                        search={{ job: j.id }}
                        className="button sm"
                        aria-label={`Open search ${title} · ${scope}`}
                      >
                        Open <ArrowRightIcon size={14} />
                      </Link>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </section>
      ) : (
        !history.error && (
          <section className="panel empty-state">
            <RavenReaction message="Nothing yet" size={48} />
            <h2>No searches yet</h2>
            <p>Run your first search to start building a history.</p>
            <Link to="/" className="button primary">
              Discover releases <ArrowRightIcon size={15} />
            </Link>
          </section>
        )
      )}
      <Pager
        offset={offset}
        total={history.data?.total}
        page={PAGE}
        onChange={setOffset}
        buttonClassName="sm"
      />
    </>
  );
}
