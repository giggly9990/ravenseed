import { Select } from "../../components/ui/select";
import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { SpinnerIcon } from "../../components/icons";
import { api } from "../../api";
import type { Family, Job, Page } from "../../types";
import { Button } from "../../components/ui/button";
import {
  RavenReaction,
  useRavenReaction,
} from "../../components/raven-reactions";
import { resolutions, ErrorNotice } from "../../app/shared";
import { FamilyTable, releaseLabel } from "./family-row";
import { FamilyDetails } from "./family-details";
import { onTabKeyDown } from "../../components/ui/tabs";
import { useDebouncedValue } from "../../hooks/use-debounced-value";

const PAGE = 20;

const initialFilters = {
  codec: "",
  resolution: "",
  releaseType: "",
  source: "",
  hdr: "",
  group: "",
  confidence: "",
  minimumCoverage: "1",
  exactOnly: false,
};
type Filters = typeof initialFilters;

const filterOptions = {
  releaseType: [
    "FULL_DISC",
    "REMUX",
    "ENCODE",
    "WEB_DL",
    "WEBRIP",
    "HDTV",
    "OTHER",
  ],
  source: ["UHD_BLURAY", "BLURAY", "WEB_DL", "WEB", "HDTV"],
  hdr: ["DV", "HDR10+", "HDR10", "SDR"],
  codec: ["HEVC", "AVC", "AV1", "MPEG2", "VC1"],
  confidence: ["EXACT", "STRONG", "POSSIBLE"],
};

const sortLabels: Record<string, string> = {
  coverage: "Coverage",
  quality: "Quality",
  seeders: "Seeders",
  size: "Size",
  newest: "Newest",
  target: "Target opportunity",
};

/** Jobs whose "got coke?" already played, so it fires once per job. */
const announced = new Set<string>();

export function Results({
  raven,
  job,
  resolution,
  setResolution,
}: {
  job: Job;
  resolution: string;
  setResolution: (value: string) => void;
  raven: ReturnType<typeof useRavenReaction>;
}) {
  const defaults = {
    ...initialFilters,
    minimumCoverage: String(job.request.filters.minimumCoverage),
  };
  const [filters, setFilters] = useState(defaults);
  const [sort, setSort] = useState("coverage"),
    [order, setOrder] = useState("desc"),
    [candidates, setCandidates] = useState(false),
    [offset, setOffset] = useState(0),
    [moreFilters, setMoreFilters] = useState(false),
    [details, setDetails] = useState<Family | null>(null);
  const opportunitiesTab = useRef<HTMLButtonElement>(null);
  // The parent keys this component by request, so a new search starts with
  // fresh filters while Refresh (a new job for the same request) keeps them.
  useEffect(() => {
    setOffset(0);
    setDetails(null);
  }, [job.id]);
  useEffect(() => {
    setOffset(0);
  }, [resolution]);
  const targetName = job.indexers.find(
    (i) => i.indexerId === job.request.targetIndexerId,
  )?.name;
  const { show, hide } = raven;
  useEffect(() => hide, [job.id, hide]);
  useEffect(() => {
    if (job.status === "running" || job.candidateCount <= 0) return;
    if (announced.has(job.id)) return;
    announced.add(job.id);
    show("opportunities", {
      anchor: opportunitiesTab.current,
      count: job.candidateCount,
      target: targetName,
      onShow: () => {
        setCandidates(true);
        setOffset(0);
        opportunitiesTab.current?.focus();
      },
    });
  }, [job.id, job.status, job.candidateCount, targetName, show]);
  // Typing in the group or coverage box waits for a pause before refetching.
  const applied = useDebouncedValue(filters);
  const params = new URLSearchParams({
    sort,
    order,
    offset: String(offset),
    limit: String(PAGE),
    ...Object.fromEntries(
      Object.entries(applied).map(([k, v]) => [k, String(v)]),
    ),
    // The server takes whole indexer counts only.
    minimumCoverage: String(
      Math.max(0, Math.trunc(Number(applied.minimumCoverage)) || 0),
    ),
    resolution,
  });
  const families = useQuery({
    queryKey: ["families", job.id, candidates, params.toString(), job.status],
    queryFn: () =>
      api<Page<Family>>(
        `/searches/${job.id}/${candidates ? "candidates" : "families"}?${params}`,
      ),
    enabled: job.status !== "running",
    // Keep this job's rows on screen while a filter change loads.
    placeholderData: (previous, query) =>
      query?.queryKey[1] === job.id ? previous : undefined,
  });
  function filter(key: keyof Filters, value: string | boolean) {
    if (key === "resolution") {
      setResolution(String(value));
      setOffset(0);
      return;
    }
    setFilters((f) => ({ ...f, [key]: value }));
    setOffset(0);
  }
  const current = (key: keyof Filters) =>
    key === "resolution" ? resolution : String(filters[key]);
  const selectFilter = (
    name: string,
    key: keyof Filters,
    options: string[],
  ) => {
    const values = current(key).split(",").filter(Boolean);
    return (
      <div
        className={`disc-filter select-compact${values.length ? " on" : ""}`}
      >
        <Select
          aria-label={name}
          multiple
          value={values.length ? values : [""]}
          triggerLabel={
            <>
              {name}
              <span className="disc-filter-value">
                {values.length ? values.map(releaseLabel).join(", ") : "any"}
              </span>
            </>
          }
          onChange={(e) =>
            filter(
              key,
              [...e.target.selectedOptions]
                .map((o) => o.value)
                .filter(Boolean)
                .join(","),
            )
          }
        >
          <option value="">Any {name.toLowerCase()}</option>
          {options.map((s) => (
            <option key={s} value={s}>
              {releaseLabel(s)}
            </option>
          ))}
        </Select>
      </div>
    );
  };
  const hiddenActive =
    (filters.releaseType ? 1 : 0) +
    (filters.codec ? 1 : 0) +
    (filters.group ? 1 : 0) +
    (filters.exactOnly ? 1 : 0);
  const anyActive =
    !!resolution ||
    Object.entries(filters).some(
      ([k, v]) => v !== defaults[k as keyof Filters],
    );
  const changeTab = (next: boolean) => {
    setCandidates(next);
    setOffset(0);
  };
  if (job.status === "running")
    return (
      <div className="panel disc-wait" role="status">
        <SpinnerIcon size={15} className="spin" />
        <span>
          Collecting the evidence. Families appear once every selected indexer
          answers or times out.
        </span>
      </div>
    );
  const total = families.data?.total ?? 0;
  return (
    <>
      <div className="disc-toolbar">
        <div role="tablist" aria-label="Result view" className="tabs">
          <button
            type="button"
            role="tab"
            className="tab"
            aria-selected={!candidates}
            aria-controls="disc-results"
            tabIndex={candidates ? -1 : 0}
            onClick={() => changeTab(false)}
            onKeyDown={(e) =>
              onTabKeyDown(e, [false, true], candidates, changeTab)
            }
          >
            All families <span className="tab-count">{job.familyCount}</span>
          </button>
          <button
            type="button"
            role="tab"
            ref={opportunitiesTab}
            className="tab"
            aria-selected={candidates}
            aria-controls="disc-results"
            tabIndex={candidates ? 0 : -1}
            onClick={() => changeTab(true)}
            onKeyDown={(e) =>
              onTabKeyDown(e, [false, true], candidates, changeTab)
            }
          >
            Opportunities{" "}
            <span
              className={`count-badge${job.candidateCount ? " accent" : ""}`}
            >
              {job.candidateCount}
            </span>
          </button>
        </div>
        <div className="disc-filters" aria-label="Filters" role="group">
          {selectFilter("Resolution", "resolution", resolutions)}
          {selectFilter("Source", "source", filterOptions.source)}
          {selectFilter("HDR", "hdr", filterOptions.hdr)}
          {selectFilter("Confidence", "confidence", filterOptions.confidence)}
          <label
            className={`disc-filter disc-filter-num${filters.minimumCoverage !== "1" ? " on" : ""}`}
            title="Fewest indexers reporting the exact same byte size"
          >
            Min coverage
            <input
              type="number"
              min={0}
              max={1000}
              step={1}
              value={filters.minimumCoverage}
              onChange={(e) => filter("minimumCoverage", e.target.value)}
            />
          </label>
          <button
            type="button"
            className="link-button disc-more"
            aria-expanded={moreFilters}
            aria-controls="disc-more-filters"
            onClick={() => setMoreFilters((open) => !open)}
          >
            {moreFilters ? "Fewer filters" : "+ More filters"}
            {!moreFilters && hiddenActive > 0 && (
              <span className="count-badge accent">{hiddenActive}</span>
            )}
          </button>
          {anyActive && (
            <button
              type="button"
              className="link-button disc-reset"
              onClick={() => {
                setFilters(defaults);
                setResolution("");
                setOffset(0);
              }}
            >
              Reset
            </button>
          )}
        </div>
        <div className="disc-sort">
          <div className="select-compact">
            <Select
              aria-label="Sort results"
              triggerLabel={
                <>
                  <span className="faint">Sort</span> {sortLabels[sort]}
                </>
              }
              value={sort}
              onChange={(e) => {
                setSort(e.target.value);
                setOffset(0);
              }}
            >
              {Object.entries(sortLabels).map(([value, text]) => (
                <option key={value} value={value}>
                  {text}
                </option>
              ))}
            </Select>
          </div>
          <div className="select-compact disc-order">
            <Select
              aria-label="Sort direction"
              triggerLabel={order === "asc" ? "↑" : "↓"}
              value={order}
              onChange={(e) => {
                setOrder(e.target.value);
                setOffset(0);
              }}
            >
              <option value="desc">Descending</option>
              <option value="asc">Ascending</option>
            </Select>
          </div>
        </div>
      </div>
      <div
        id="disc-more-filters"
        className="disc-more-filters"
        hidden={!moreFilters}
      >
        {selectFilter("Release type", "releaseType", filterOptions.releaseType)}
        {selectFilter("Codec", "codec", filterOptions.codec)}
        <label className="disc-filter disc-filter-text">
          Group
          <input
            placeholder="Any group"
            value={filters.group}
            onChange={(e) => filter("group", e.target.value)}
          />
        </label>
        <label
          className="check-row small"
          title="Exact means matching reported metadata and a positive byte size. Payload verification is still needed."
        >
          <input
            type="checkbox"
            checked={filters.exactOnly}
            onChange={(e) => filter("exactOnly", e.target.checked)}
          />
          Exact-size families only
        </label>
        <span className="hint">
          Filters apply to results already collected; choose several values per
          filter.
        </span>
      </div>
      <ErrorNotice error={families.error} />
      <div id="disc-results" role="tabpanel" className="disc-results">
        {families.isLoading ? (
          <div className="panel disc-wait" role="status">
            <SpinnerIcon className="spin" size={15} /> Loading families
          </div>
        ) : families.isError ? null : !families.data?.items.length ? (
          <div className="panel empty-results">
            <RavenReaction message="xD" crossedEye size={48} />
            <h3>No {candidates ? "opportunities" : "families"} found</h3>
            <p>
              {job.familyCount
                ? "Try a lower minimum coverage or clear the filters."
                : "No matching releases were returned for this search."}
            </p>
          </div>
        ) : (
          <FamilyTable
            jobId={job.id}
            families={families.data.items}
            selected={job.request.indexerIds.length}
            targetName={job.request.targetIndexerId ? targetName : undefined}
            onDetails={setDetails}
          />
        )}
      </div>
      {families.data && total > 0 && (
        <div className="pagination disc-pagination">
          <span>
            {total > PAGE
              ? `${offset + 1}–${Math.min(offset + PAGE, total)} of ${total}`
              : `${total}`}{" "}
            {candidates ? "opportunities" : "families"} ·{" "}
            <span className="num">{job.resultCount}</span> raw results ·
            coverage counts indexers reporting the exact same byte size.
          </span>
          {total > PAGE && (
            <>
              <Button
                className="sm"
                disabled={offset === 0}
                onClick={() => setOffset((o) => Math.max(0, o - PAGE))}
              >
                Previous
              </Button>
              <Button
                className="sm"
                disabled={offset + PAGE >= total}
                onClick={() => setOffset((o) => o + PAGE)}
              >
                Next
              </Button>
            </>
          )}
        </div>
      )}
      <FamilyDetails
        jobId={job.id}
        family={details}
        showFamily={setDetails}
        close={() => setDetails(null)}
      />
    </>
  );
}
