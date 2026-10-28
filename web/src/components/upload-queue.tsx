import { Fragment, useCallback, useState } from "react";
import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { api, safeURL } from "../api";
import type { TrackerResult, UploadJob, UploadJobsPage } from "../types";
import { Button } from "./ui/button";
import { Pager } from "./ui/pager";
import { onTabKeyDown } from "./ui/tabs";
import { ChevronDownIcon, ChevronRightIcon } from "./icons";
import { RavenError } from "./raven-reactions";
import { ReuploadDialog } from "./reupload-dialog";
import { relativeTime } from "../lib/time";
import { useNow } from "../hooks/use-now";

const PAGE = 20;

type Tab = "active" | "needs" | "done";
const tabs: { id: Tab; label: string; empty: string }[] = [
  { id: "active", label: "Active", empty: "Nothing is running right now." },
  { id: "needs", label: "Needs you", empty: "Nothing needs your attention." },
  { id: "done", label: "Done", empty: "No finished uploads yet." },
];

const needsYou = new Set(["failed", "needs_review"]);
const done = new Set(["finished", "cancelled"]);
const retryable = new Set(["failed", "needs_review", "finished", "cancelled"]);

/** Statuses the queue's Active tab shows (the sidebar badge counts these). */
export const activeUploadStatuses = ["adding", "downloading", "uploading"];

/** Server-side status filter of each tab. */
const tabStatuses: Record<Tab, string[]> = {
  active: activeUploadStatuses,
  needs: [...needsYou],
  done: [...done],
};

function tabOf(status: string): Tab {
  if (needsYou.has(status)) return "needs";
  if (done.has(status)) return "done";
  return "active";
}

/** Jobs per tab, from the server's per-status counts of the whole queue. */
function tabCounts(statusCounts: Record<string, number> | undefined) {
  const counts: Record<Tab, number> = { active: 0, needs: 0, done: 0 };
  for (const [status, n] of Object.entries(statusCounts ?? {}))
    counts[tabOf(status)] += n;
  return counts;
}

const pillTone: Record<string, string> = {
  uploading: "soft",
  downloading: "info",
  adding: "dashed",
  needs_review: "warn",
  failed: "danger",
  finished: "ok",
  cancelled: "quiet",
};

const statusLabel = (status: string) => {
  const text = status.replaceAll("_", " ");
  return text.charAt(0).toUpperCase() + text.slice(1);
};

/** A finished or failed job's pill names what the tracker did, when UA reported it. */
const trackerPill: Record<string, { label: string; tone: string }> = {
  success: { label: "Uploaded", tone: "ok" },
  dupe: { label: "Dupe", tone: "danger" },
  skipped: { label: "Skipped", tone: "danger" },
  banned: { label: "Banned", tone: "danger" },
};

function pillOf(job: UploadJob) {
  const tracker =
    (job.status === "finished" || job.status === "failed") && job.trackerResult
      ? trackerPill[job.trackerResult.status]
      : undefined;
  return (
    tracker ?? {
      label: statusLabel(job.status),
      tone: pillTone[job.status] ?? "quiet",
    }
  );
}

const trackerLabel = (result: TrackerResult) =>
  trackerPill[result.status]?.label ?? statusLabel(result.status);

const errorSummary: Record<string, string> = {
  failed: "Upload did not finish",
  needs_review: "Check qui and Upload-Assistant first",
};

/** A failed job the tracker turned away was never uploaded; say why. */
const notUploaded: Record<string, string> = {
  dupe: "Not uploaded: possible dupe",
  banned: "Not uploaded: release group banned",
  skipped: "Not uploaded: skipped",
};

function errorOf(job: UploadJob) {
  return (
    (job.status === "failed" && notUploaded[job.trackerResult?.status ?? ""]) ||
    (errorSummary[job.status] ?? job.message)
  );
}

function summarize(statusCounts: Record<string, number>) {
  const count = (statuses: Iterable<string>) =>
    [...statuses].reduce((sum, status) => sum + (statusCounts[status] ?? 0), 0);
  const parts = [
    [count(["uploading"]), "uploading"],
    [count(["downloading"]), "downloading"],
    [count(["adding"]), "waiting"],
    [count(needsYou), "need you"],
  ] as const;
  const text = parts
    .filter(([n]) => n > 0)
    .map(([n, label]) => `${n} ${label}`)
    .join(" · ");
  return text || "Nothing running";
}

function attemptInfo(job: UploadJob) {
  const parts = [`Attempt ${(job.attempt ?? 0) + 1}`];
  const previous = job.previousAttempts?.at(-1);
  if (previous)
    parts.push(
      `previous attempt ${previous.status.replaceAll("_", " ")}: ${previous.message}`,
    );
  return parts.join(" · ");
}

export function UploadQueue() {
  const [chosenTab, setTab] = useState<Tab | null>(null);
  const [paging, setPaging] = useState<{ tab: Tab; offset: number }>({
    tab: "active",
    offset: 0,
  });
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [retryJob, setRetryJob] = useState<UploadJob | null>(null);
  const qc = useQueryClient();
  // Every response counts the whole queue by status. The last one (kept
  // while the next tab or page loads) picks the tab and its totals.
  const [lastCounts, setLastCounts] = useState<Record<string, number>>();
  const counts = tabCounts(lastCounts);
  // Until the user picks a tab, open the first one with something in it.
  const tab: Tab =
    chosenTab ??
    (counts.active
      ? "active"
      : counts.needs
        ? "needs"
        : counts.done
          ? "done"
          : "active");
  const offset = paging.tab === tab ? paging.offset : 0;
  const setOffset = useCallback(
    (next: number) => setPaging({ tab, offset: next }),
    [tab],
  );
  const queue = useQuery({
    queryKey: ["upload-queue", tab, offset],
    queryFn: async () => {
      const page = await api<UploadJobsPage>(
        `/uploads?limit=${PAGE}&offset=${offset}&status=${tabStatuses[tab].join(",")}`,
      );
      setLastCounts(page.statusCounts);
      return page;
    },
    placeholderData: keepPreviousData,
    // Fast while anything in the queue is moving, slow when all is settled.
    refetchInterval: (query) =>
      tabCounts(query.state.data?.statusCounts).active ? 5000 : 30000,
  });
  // Re-renders so "updated 2 minutes ago" keeps moving between refetches.
  useNow();
  const cancel = useMutation({
    mutationFn: (id: string) =>
      api(`/uploads/${id}/cancel`, { method: "POST", body: "{}" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["upload-queue"] }),
  });

  // Rows of the previous tab or page are not shown while the next loads.
  const loading = queue.isPlaceholderData;
  const visible = loading ? [] : (queue.data?.items ?? []);
  const total = counts.active + counts.needs + counts.done;
  const toggle = (id: string) =>
    setExpanded((current) => {
      const next = new Set(current);
      if (!next.delete(id)) next.add(id);
      return next;
    });
  const reupload = (j: UploadJob, className = "button sm") => (
    <Button className={className} onClick={() => setRetryJob(j)}>
      Reupload
    </Button>
  );

  return (
    <>
      <header className="page-header upl-header">
        <h1>Upload queue</h1>
        {lastCounts && (
          <span className="muted small" aria-live="polite">
            {summarize(lastCounts)}
          </span>
        )}
        {queue.data && total > 0 && (
          <div
            className="tabs upl-tabs"
            role="tablist"
            aria-label="Filter uploads"
          >
            {tabs.map((t) => (
              <button
                key={t.id}
                type="button"
                role="tab"
                id={`upl-tab-${t.id}`}
                aria-selected={tab === t.id}
                aria-controls="upl-panel"
                tabIndex={tab === t.id ? 0 : -1}
                onKeyDown={(e) =>
                  onTabKeyDown(
                    e,
                    tabs.map((x) => x.id),
                    tab,
                    setTab,
                  )
                }
                className="tab"
                onClick={() => setTab(t.id)}
              >
                {t.label}{" "}
                <span
                  className={`tab-count${t.id === "needs" && counts.needs ? " upl-hot" : ""}`}
                >
                  {counts[t.id]}
                </span>
              </button>
            ))}
          </div>
        )}
      </header>
      <p className="hint">
        Waiting jobs survive restarts. Interrupted submissions need review and
        are never resubmitted automatically.
        {/* Older Upload-Assistant versions report no tracker result. */}
        {visible.some((j) => j.status === "finished" && !j.trackerResult) &&
          " Finished without a tracker result means UA exited cleanly; check its WebUI for the tracker outcome."}
      </p>
      {(queue.error || cancel.error) && (
        <RavenError>{(queue.error || cancel.error)?.message}</RavenError>
      )}
      {queue.isPending && (
        <p role="status" className="muted">
          Loading upload queue...
        </p>
      )}
      {queue.data && total === 0 && (
        <section className="panel empty-state">
          <h2>No uploads yet</h2>
          <p>
            Turn on Upload-Assistant in{" "}
            <Link to="/settings" search={{ section: "uploads" }}>
              Settings → Upload-Assistant
            </Link>
            , then pick Upload to target when adding a torrent.
          </p>
        </section>
      )}
      {queue.data && total > 0 && (
        <section
          className="panel table-wrap"
          id="upl-panel"
          role="tabpanel"
          aria-labelledby={`upl-tab-${tab}`}
        >
          <table className="data-table upl-table">
            <colgroup>
              <col />
              <col className="upl-col-to" />
              <col className="upl-col-status" />
              <col className="upl-col-preset" />
              <col className="upl-col-updated" />
              <col className="upl-col-actions" />
            </colgroup>
            <thead>
              <tr>
                <th scope="col">Release</th>
                <th scope="col">To</th>
                <th scope="col">Status</th>
                <th scope="col">Preset</th>
                <th scope="col">Updated</th>
                <th scope="col">
                  <span className="visually-hidden">Actions</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {visible.length === 0 && (
                <tr>
                  <td colSpan={6} className="table-empty">
                    {loading
                      ? "Loading..."
                      : tabs.find((t) => t.id === tab)?.empty}
                  </td>
                </tr>
              )}
              {visible.map((j) => {
                const open = expanded.has(j.id);
                const failed = needsYou.has(j.status);
                const pill = pillOf(j);
                const link = safeURL(j.trackerResult?.link ?? "");
                const updated = j.updatedAt || j.createdAt;
                const percent = Math.round(
                  Math.min(1, Math.max(0, j.progress || 0)) * 100,
                );
                return (
                  <Fragment key={j.id}>
                    <tr className={failed ? "upl-has-error" : undefined}>
                      <td>
                        <span className="cell-title truncate" title={j.title}>
                          {j.title}
                        </span>
                        <span
                          className="cell-sub truncate"
                          title={failed ? undefined : j.message}
                        >
                          {failed ? errorOf(j) : j.message}
                        </span>
                      </td>
                      <td className="upl-to">
                        {link ? (
                          <a
                            href={link}
                            target="_blank"
                            rel="noreferrer noopener"
                            title={`Open on ${j.tracker}`}
                          >
                            {j.tracker}
                          </a>
                        ) : (
                          j.tracker
                        )}
                      </td>
                      <td>
                        <div className="upl-status">
                          <span className={`pill ${pill.tone}`}>
                            {pill.label}
                            {j.status === "downloading" && (
                              <span className="upl-pct">{percent}%</span>
                            )}
                          </span>
                          {j.status === "downloading" && (
                            <span
                              className="bar info"
                              role="progressbar"
                              aria-label="Download progress"
                              aria-valuemin={0}
                              aria-valuemax={100}
                              aria-valuenow={percent}
                            >
                              <span style={{ width: `${percent}%` }} />
                            </span>
                          )}
                        </div>
                      </td>
                      <td
                        className="small muted truncate"
                        title={j.flagPresetName || j.args || undefined}
                      >
                        {j.flagPresetName || (j.args ? "Custom flags" : "—")}
                      </td>
                      <td className="small faint">
                        <time
                          dateTime={updated}
                          title={new Date(updated).toLocaleString()}
                        >
                          {relativeTime(updated)}
                        </time>
                      </td>
                      <td className="actions">
                        <div className="upl-actions">
                          {j.status === "downloading" && (
                            <Button
                              className="button sm"
                              disabled={cancel.isPending}
                              onClick={() => cancel.mutate(j.id)}
                            >
                              Cancel upload
                            </Button>
                          )}
                          {retryable.has(j.status) && !failed && reupload(j)}
                          <button
                            type="button"
                            className="icon-button"
                            aria-expanded={open}
                            aria-controls={`upl-details-${j.id}`}
                            aria-label={`Details for ${j.title}`}
                            title="Job details"
                            onClick={() => toggle(j.id)}
                          >
                            {open ? (
                              <ChevronDownIcon size={16} />
                            ) : (
                              <ChevronRightIcon size={16} />
                            )}
                          </button>
                        </div>
                      </td>
                    </tr>
                    {failed && (
                      <tr className="upl-error-row">
                        <td colSpan={6}>
                          <RavenError
                            action={reupload(j, "button sm upl-error-button")}
                          >
                            <strong className="upl-error-title">
                              {j.message}
                            </strong>
                            <span className="upl-error-sub">
                              {attemptInfo(j)}
                            </span>
                          </RavenError>
                        </td>
                      </tr>
                    )}
                    {open && (
                      <tr
                        className="upl-details-row"
                        id={`upl-details-${j.id}`}
                      >
                        <td colSpan={6}>
                          <JobDetails job={j} />
                        </td>
                      </tr>
                    )}
                  </Fragment>
                );
              })}
            </tbody>
          </table>
        </section>
      )}
      {retryJob && (
        <ReuploadDialog job={retryJob} close={() => setRetryJob(null)} />
      )}
      <Pager
        offset={offset}
        total={loading ? undefined : queue.data?.total}
        page={PAGE}
        onChange={setOffset}
        buttonClassName="sm"
      />
    </>
  );
}

function JobDetails({ job: j }: { job: UploadJob }) {
  const link = safeURL(j.trackerResult?.link ?? "");
  return (
    <div className="upl-details">
      <dl className="upl-dl">
        <dt>Session</dt>
        <dd className="mono">
          {j.id}
          {j.attempt ? `_retry_${j.attempt}` : ""}
        </dd>
        <dt>Attempt</dt>
        <dd>{(j.attempt ?? 0) + 1}</dd>
        <dt>Instance</dt>
        <dd>{j.instanceId}</dd>
        <dt>Created</dt>
        <dd>{new Date(j.createdAt).toLocaleString()}</dd>
        {!!j.media?.tmdbId && (
          <>
            <dt>TMDb</dt>
            <dd className="mono">
              {j.media.type}/{j.media.tmdbId}
            </dd>
          </>
        )}
        {j.flagPresetName && (
          <>
            <dt>UA preset</dt>
            <dd>{j.flagPresetName}</dd>
          </>
        )}
        {j.args && (
          <>
            <dt>UA flags</dt>
            <dd>
              <code className="mono break">{j.args}</code>
            </dd>
          </>
        )}
        {j.hash && (
          <>
            <dt>Hash</dt>
            <dd className="mono break">{j.hash}</dd>
          </>
        )}
        {j.contentPath && (
          <>
            <dt>Download</dt>
            <dd className="mono break">{j.contentPath}</dd>
          </>
        )}
        {j.uploadPath && (
          <>
            <dt>Upload-Assistant</dt>
            <dd className="mono break">{j.uploadPath}</dd>
          </>
        )}
        {j.trackerResult?.torrentId && (
          <>
            <dt>Torrent ID</dt>
            <dd className="mono">{j.trackerResult.torrentId}</dd>
          </>
        )}
        {link && (
          <>
            <dt>On tracker</dt>
            <dd className="break">
              <a href={link} target="_blank" rel="noreferrer noopener">
                {link}
              </a>
            </dd>
          </>
        )}
      </dl>
      {!!j.previousAttempts?.length && (
        <div className="upl-attempts">
          <h3>Previous attempts</h3>
          <ol>
            {j.previousAttempts.map((attempt) => (
              <li key={attempt.attempt}>
                <span>
                  Attempt {attempt.attempt + 1}:{" "}
                  <strong>{attempt.status.replaceAll("_", " ")}</strong>
                  {attempt.trackerResult && (
                    <TrackerOutcome result={attempt.trackerResult} />
                  )}
                  {attempt.updatedAt && (
                    <span className="faint">
                      {" "}
                      · {relativeTime(attempt.updatedAt)}
                    </span>
                  )}
                </span>
                <span className="muted">{attempt.message}</span>
                {(attempt.args || !!attempt.media.tmdbId) && (
                  <span className="faint">
                    {attempt.args && (
                      <>
                        UA flags: <code className="mono">{attempt.args}</code>
                      </>
                    )}
                    {attempt.args && !!attempt.media.tmdbId && " · "}
                    {!!attempt.media.tmdbId &&
                      `TMDb: ${attempt.media.type}/${attempt.media.tmdbId}`}
                  </span>
                )}
              </li>
            ))}
          </ol>
        </div>
      )}
    </div>
  );
}

/** A previous attempt's tracker result, linked when the link is safe. */
function TrackerOutcome({ result }: { result: TrackerResult }) {
  const link = safeURL(result.link ?? "");
  const text = `tracker: ${trackerLabel(result)}`;
  return (
    <>
      {" · "}
      {link ? (
        <a href={link} target="_blank" rel="noreferrer noopener">
          {text}
        </a>
      ) : (
        text
      )}
    </>
  );
}
