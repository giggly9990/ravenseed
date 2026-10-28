import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { api, bytes } from "../../api";
import type { Monitor, MonitorCandidate, Page, QuiInstance } from "../../types";
import { Button } from "../../components/ui/button";
import { Pager } from "../../components/ui/pager";
import { Select } from "../../components/ui/select";
import { ErrorNotice, resolutions } from "../../app/shared";
import { FilterIcon, PlusIcon, SearchIcon } from "../../components/icons";
import { useRavenReaction } from "../../components/raven-reactions";
import {
  candidateClasses,
  candidateCountKey,
  uploadableClasses,
  uploadableCount,
  useCandidateCounts,
  type CandidateClass,
} from "../../hooks/use-candidate-counts";
import { useDebouncedValue } from "../../hooks/use-debounced-value";
import { CreateMonitorDialog } from "./create-monitor-dialog";
import { SendToUA } from "./send-to-ua";
import { MonitorCard } from "./monitor-card";
import { CandidateDetail } from "./candidate-detail";
import { canSendToUA, candidatePill, classLabels } from "./format";
import { shortRelativeTime } from "../../lib/time";

const PAGE = 50;
const releaseClasses = [
  "FULL_DISC",
  "REMUX",
  "ENCODE",
  "WEB_DL",
  "WEBRIP",
  "HDTV",
  "OTHER",
];

export function MonitorsPage() {
  const qc = useQueryClient();
  const raven = useRavenReaction();
  const [creating, setCreating] = useState(false);
  const [active, setActive] = useState<string | null>(null);
  const [offset, setOffset] = useState(0);
  const [classification, setClassification] = useState<CandidateClass>("all");
  const [releaseClass, setReleaseClass] = useState("");
  const [resolution, setResolution] = useState("");
  const [sourceFilter, setSourceFilter] = useState("");
  const [hdr, setHdr] = useState("");
  const [group, setGroup] = useState("");
  const [policyVersion, setPolicyVersion] = useState("");
  const [text, setText] = useState("");
  /** "" keeps the server's default order (pending work first). */
  const [checkedSort, setCheckedSort] = useState<
    "" | "checked_desc" | "checked_asc"
  >("");
  /** Debounced name search, applied by the server across every page. */
  const [search, setSearch] = useState("");
  const [moreFilters, setMoreFilters] = useState(false);
  const [selectedCandidate, setSelectedCandidate] = useState<string | null>(
    null,
  );
  const [uploadCandidate, setUploadCandidate] =
    useState<MonitorCandidate | null>(null);
  const missingChip = useRef<HTMLButtonElement | null>(null);
  const cardNodes = useRef(new Map<string, HTMLDivElement>());

  const monitors = useQuery({
    queryKey: ["monitors"],
    queryFn: () => api<Monitor[]>("/monitors"),
    refetchInterval: 30000,
  });
  const instances = useQuery({
    queryKey: ["qui-instances"],
    queryFn: () => api<QuiInstance[]>("/qui/instances"),
  });
  // One request counts every class of every monitor (cards and chips).
  const counts = useCandidateCounts(monitors.data);
  const activeId = active ?? monitors.data?.[0]?.id ?? null;
  const activeCounts = activeId ? counts[activeId] : undefined;
  const selected = monitors.data?.find((m) => m.id === activeId);

  const extraFilters = {
    releaseClass,
    resolution,
    source: sourceFilter,
    hdr,
    group,
    policy: policyVersion,
  };
  const extraCount = Object.values(extraFilters).filter(Boolean).length;
  // The free-text filters refetch after a pause in typing, not per keystroke.
  const appliedFilters = useDebouncedValue(extraFilters);
  useEffect(() => {
    const next = text.trim();
    if (next === search) return;
    const timer = setTimeout(() => {
      setSearch(next);
      setOffset(0);
    }, 300);
    return () => clearTimeout(timer);
  }, [text, search]);
  const candidates = useQuery({
    queryKey: [
      "monitor-candidates",
      activeId,
      offset,
      classification,
      appliedFilters,
      search,
      checkedSort,
    ],
    queryFn: () => {
      const query = new URLSearchParams({
        limit: String(PAGE),
        offset: String(offset),
        classification,
      });
      for (const [key, value] of Object.entries(appliedFilters)) {
        if (value) query.set(key, value);
      }
      if (search) query.set("q", search);
      if (checkedSort) query.set("sort", checkedSort);
      return api<Page<MonitorCandidate>>(
        `/monitors/${activeId}/candidates?${query}`,
      );
    },
    enabled: !!activeId,
    refetchInterval: 10000,
    // Keep this monitor's rows on screen while a filter or page loads.
    placeholderData: (previous, query) =>
      query?.queryKey[1] === activeId ? previous : undefined,
  });

  /** Refreshes the counts; one request covers every monitor. */
  const refreshCounts = () =>
    void qc.invalidateQueries({ queryKey: candidateCountKey });
  const remove = useMutation({
    mutationFn: (id: string) => api(`/monitors/${id}`, { method: "DELETE" }),
    onSuccess: (_, id) => {
      if (id === activeId) selectMonitor(null);
      // Drop it now: until the refetch lands it would otherwise become the
      // fallback selection and request candidates of a deleted monitor.
      qc.setQueryData<Monitor[]>(["monitors"], (items) =>
        items?.filter((item) => item.id !== id),
      );
      void qc.invalidateQueries({ queryKey: ["monitors"] });
    },
  });
  const setState = useMutation({
    mutationFn: ({ id, enabled }: { id: string; enabled: boolean }) =>
      api<Monitor>(`/monitors/${id}`, {
        method: "PUT",
        body: JSON.stringify({ enabled }),
      }),
    onSuccess: (updated) => {
      qc.setQueryData<Monitor[]>(["monitors"], (items) =>
        items?.map((item) => (item.id === updated.id ? updated : item)),
      );
      void qc.invalidateQueries({ queryKey: ["monitors"] });
    },
  });
  const scan = useMutation({
    mutationFn: (id: string) =>
      api(`/monitors/${id}/scan`, { method: "POST", body: "{}" }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["monitors"] });
      void qc.invalidateQueries({ queryKey: ["monitor-candidates"] });
      refreshCounts();
    },
  });
  const check = useMutation({
    mutationFn: (id: string) =>
      api(`/monitor-candidates/${id}/check`, { method: "POST", body: "{}" }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["monitor-candidates"] });
      refreshCounts();
    },
  });

  function selectMonitor(id: string | null) {
    setActive(id);
    setOffset(0);
    setSelectedCandidate(null);
  }
  function filterBy(next: CandidateClass) {
    setClassification(next);
    setOffset(0);
  }
  function setExtra(setter: (v: string) => void, value: string) {
    setter(value);
    setOffset(0);
  }

  // got coke?: the uploadable count for the selected monitor went up.
  const countsKnown =
    !!activeCounts &&
    uploadableClasses.every((c) => activeCounts[c] !== undefined);
  const uploadable = countsKnown ? uploadableCount(activeCounts) : undefined;
  const baseline = useRef<{ id: string | null; value?: number }>({ id: null });
  const { show } = raven;
  useEffect(() => {
    const previous = baseline.current;
    if (previous.id !== activeId) {
      baseline.current = { id: activeId, value: uploadable };
      return;
    }
    if (uploadable === undefined) return;
    if (previous.value !== undefined && uploadable > previous.value) {
      const anchor =
        missingChip.current ??
        (activeId ? cardNodes.current.get(activeId) : undefined);
      show("opportunities", {
        anchor,
        count: uploadable - previous.value,
        target: selected?.targetIndexerName,
        onShow: () => filterBy("missing_slot"),
      });
    }
    baseline.current = { id: activeId, value: uploadable };
  }, [activeId, uploadable, show, selected?.targetIndexerName]);

  const instanceName = (id: number) =>
    instances.data?.find((i) => i.id === id)?.name;
  const rows = candidates.data?.items ?? [];
  const detail = candidates.data?.items.find((c) => c.id === selectedCandidate);
  // The dialog follows refetches of the candidate it opened with (SendToUA
  // relies on that), falling back to the snapshot once it leaves the page.
  const sending =
    uploadCandidate &&
    (candidates.data?.items.find((c) => c.id === uploadCandidate.id) ??
      uploadCandidate);

  return (
    <>
      <header className="page-header">
        <h1>Tracker monitors</h1>
        <p>
          Find torrents you already seed on one tracker that the other is
          missing.
        </p>
        <div className="actions">
          <Button className="primary" onClick={() => setCreating(true)}>
            <PlusIcon size={14} />
            New monitor
          </Button>
        </div>
      </header>

      {monitors.data?.length === 0 && (
        <div className="panel empty-state">
          <h2>No tracker pairs yet</h2>
          <p>
            Pair a tracker you seed on with one you upload to, and RavenSeed
            checks your qui torrents against it.
          </p>
          <Button className="primary" onClick={() => setCreating(true)}>
            <PlusIcon size={14} />
            New monitor
          </Button>
        </div>
      )}

      {!!monitors.data?.length && (
        <div className="mon-cards" role="list" aria-label="Monitors">
          {monitors.data.map((m) => (
            <MonitorCard
              key={m.id}
              monitor={m}
              instances={instances.data}
              counts={counts[m.id] ?? {}}
              selected={m.id === activeId}
              select={() => {
                if (m.id !== activeId) selectMonitor(m.id);
              }}
              scan={() => scan.mutate(m.id)}
              scanning={scan.isPending && scan.variables === m.id}
              toggle={() => setState.mutate({ id: m.id, enabled: !m.enabled })}
              toggling={setState.isPending}
              remove={() => remove.mutate(m.id)}
              removing={remove.isPending}
              cardRef={(node) => {
                if (node) cardNodes.current.set(m.id, node);
                else cardNodes.current.delete(m.id);
              }}
            />
          ))}
        </div>
      )}
      <ErrorNotice
        error={monitors.error || scan.error || setState.error || remove.error}
      />

      {selected && (
        <div className="mon-workspace">
          <section
            className="panel mon-candidates"
            aria-label={`${selected.sourceIndexerName} to ${selected.targetIndexerName} candidates`}
          >
            <div
              className="chips mon-chips"
              role="group"
              aria-label="Classification"
            >
              {candidateClasses.map((c) => {
                const n = activeCounts?.[c];
                const hot = uploadableClasses.includes(c) && (n ?? 0) > 0;
                return (
                  <button
                    key={c}
                    type="button"
                    className="chip"
                    aria-pressed={classification === c}
                    ref={c === "missing_slot" ? missingChip : undefined}
                    onClick={() => filterBy(c)}
                  >
                    {classLabels[c]}
                    <span className={`chip-count${hot ? " hot" : ""}`}>
                      {n ?? "–"}
                    </span>
                  </button>
                );
              })}
            </div>
            <div className="mon-toolbar">
              <label className="mon-search">
                <SearchIcon size={14} aria-hidden="true" />
                <input
                  aria-label="Filter by name or group"
                  placeholder="Name or group"
                  value={text}
                  onChange={(e) => setText(e.target.value)}
                />
              </label>
              <button
                type="button"
                className="button sm quiet"
                aria-expanded={moreFilters}
                aria-controls="mon-more-filters"
                onClick={() => setMoreFilters(!moreFilters)}
              >
                <FilterIcon size={13} />
                More filters
                {extraCount > 0 && (
                  <span className="count-badge accent">{extraCount}</span>
                )}
              </button>
              {selected.enabled ? (
                <span className="mon-queue">
                  Queue: 1 check/min · rechecks every {selected.intervalHours} h
                </span>
              ) : (
                <span className="mon-queue" role="status">
                  Rechecks paused. Queued searches and scans continue when you
                  resume.
                </span>
              )}
            </div>
            {moreFilters && (
              <div className="mon-more" id="mon-more-filters">
                <label>
                  Release class
                  <Select
                    aria-label="Release class filter"
                    value={releaseClass}
                    onChange={(e) => setExtra(setReleaseClass, e.target.value)}
                  >
                    <option value="">Any class</option>
                    {releaseClasses.map((value) => (
                      <option key={value} value={value}>
                        {value.replaceAll("_", " ")}
                      </option>
                    ))}
                  </Select>
                </label>
                <label>
                  Resolution
                  <Select
                    aria-label="Resolution filter"
                    value={resolution}
                    onChange={(e) => setExtra(setResolution, e.target.value)}
                  >
                    <option value="">Any resolution</option>
                    {resolutions.map((value) => (
                      <option key={value} value={value}>
                        {value}
                      </option>
                    ))}
                  </Select>
                </label>
                <label>
                  Source
                  <input
                    value={sourceFilter}
                    onChange={(e) =>
                      setExtra(setSourceFilter, e.target.value.toUpperCase())
                    }
                    placeholder="WEB_DL"
                  />
                </label>
                <label>
                  HDR
                  <input
                    value={hdr}
                    onChange={(e) =>
                      setExtra(setHdr, e.target.value.toUpperCase())
                    }
                    placeholder="HDR10"
                  />
                </label>
                <label>
                  Group
                  <input
                    value={group}
                    onChange={(e) => setExtra(setGroup, e.target.value)}
                    placeholder="Exact group"
                  />
                </label>
                <label>
                  Policy version
                  <input
                    value={policyVersion}
                    onChange={(e) => setExtra(setPolicyVersion, e.target.value)}
                    placeholder="policy_…"
                  />
                </label>
                {extraCount > 0 && (
                  <button
                    type="button"
                    className="link-button small"
                    onClick={() => {
                      for (const set of [
                        setReleaseClass,
                        setResolution,
                        setSourceFilter,
                        setHdr,
                        setGroup,
                        setPolicyVersion,
                      ])
                        set("");
                      setOffset(0);
                    }}
                  >
                    Clear filters
                  </button>
                )}
              </div>
            )}
            <div className="table-wrap">
              <table className="data-table mon-table">
                <thead>
                  <tr>
                    <th scope="col">Release on {selected.sourceIndexerName}</th>
                    <th scope="col" className="num">
                      Size
                    </th>
                    <th scope="col">On {selected.targetIndexerName}</th>
                    <th
                      scope="col"
                      aria-sort={
                        checkedSort === "checked_desc"
                          ? "descending"
                          : checkedSort === "checked_asc"
                            ? "ascending"
                            : "none"
                      }
                    >
                      <button
                        type="button"
                        className="mon-sort"
                        onClick={() => {
                          setCheckedSort(
                            checkedSort === ""
                              ? "checked_desc"
                              : checkedSort === "checked_desc"
                                ? "checked_asc"
                                : "",
                          );
                          setOffset(0);
                        }}
                      >
                        Checked
                        <span aria-hidden="true">
                          {checkedSort === "checked_desc"
                            ? " ↓"
                            : checkedSort === "checked_asc"
                              ? " ↑"
                              : ""}
                        </span>
                      </button>
                    </th>
                    <th scope="col" className="actions">
                      <span className="visually-hidden">Actions</span>
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((c) => {
                    const pill = candidatePill(c);
                    const isSelected = c.id === selectedCandidate;
                    return (
                      <tr
                        key={c.id}
                        className={isSelected ? "selected" : undefined}
                        onClick={(e) => {
                          if ((e.target as Element).closest("a,button")) return;
                          setSelectedCandidate(c.id);
                        }}
                      >
                        <td className="mon-release">
                          <button
                            type="button"
                            className="mon-row-select"
                            aria-pressed={isSelected}
                            onClick={() => setSelectedCandidate(c.id)}
                          >
                            <span className="cell-title truncate">
                              {c.name}
                            </span>
                          </button>
                          {c.message && (
                            <span className="cell-sub truncate">
                              {c.message}
                            </span>
                          )}
                        </td>
                        <td className="num small">{bytes(c.size)}</td>
                        <td>
                          <span className={`pill ${pill.tone}`}>
                            {pill.label}
                          </span>
                        </td>
                        <td className="small faint mon-nowrap">
                          {c.lastCheckedAt
                            ? shortRelativeTime(c.lastCheckedAt)
                            : "pending"}
                        </td>
                        <td className="actions">
                          {canSendToUA(c) && (
                            <Button
                              className="soft sm"
                              onClick={() => setUploadCandidate(c)}
                            >
                              Send to UA
                            </Button>
                          )}
                          {c.uploadJobId && (
                            <Link to="/uploads" className="small">
                              View upload →
                            </Link>
                          )}
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
              {candidates.data && rows.length === 0 && (
                <p className="table-empty">
                  {classification === "all" && extraCount === 0 && !search
                    ? `No torrents found for ${selected.sourceTrackerHost} yet.`
                    : "Nothing matches these filters."}
                </p>
              )}
            </div>
            <ErrorNotice error={candidates.error || check.error} />
            <Pager
              className="panel-foot"
              offset={offset}
              total={candidates.data?.total}
              page={PAGE}
              onChange={setOffset}
              showSinglePage
            />
          </section>
          <CandidateDetail
            candidate={detail}
            monitor={selected}
            instanceName={detail ? instanceName(detail.instanceId) : undefined}
            close={() => setSelectedCandidate(null)}
            send={() => detail && setUploadCandidate(detail)}
            check={() => detail && check.mutate(detail.id)}
            checking={check.isPending}
          />
        </div>
      )}

      {creating && (
        <CreateMonitorDialog
          close={() => setCreating(false)}
          created={(monitor) => {
            selectMonitor(monitor.id);
            setCreating(false);
          }}
        />
      )}
      {sending && (
        <SendToUA
          candidate={sending}
          targetIndexerId={selected?.targetIndexerId ?? 0}
          targetName={selected?.targetIndexerName}
          instanceName={instanceName(sending.instanceId)}
          close={() => setUploadCandidate(null)}
        />
      )}
      {raven.reaction}
    </>
  );
}
