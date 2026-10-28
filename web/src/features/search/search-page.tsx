import { Select } from "../../components/ui/select";
import { useEffect, useRef, useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import {
  ArrowRightIcon,
  ConnectionIcon,
  SearchIcon,
  SpinnerIcon,
} from "../../components/icons";
import { api, parseEvents } from "../../api";
import type { Indexer, Job, SearchRequest, Settings } from "../../types";
import { Button } from "../../components/ui/button";
import { IndexerPresets } from "../../components/indexer-presets";
import { useRavenReaction } from "../../components/raven-reactions";
import {
  parseMediaInput,
  useMediaPreview,
} from "../../components/media-metadata";
import { resolutions, ErrorNotice } from "../../app/shared";
import { Results } from "./search-results";
import { TitleStrip } from "./title-strip";
import { JobStatus, PendingSeeds } from "./job-status";
import { searchRoute } from "../../app/router";

function useJob(id?: string) {
  const qc = useQueryClient();
  /** True while the event stream is connected; polling is only a fallback. */
  const [streaming, setStreaming] = useState(false);
  const query = useQuery({
    queryKey: ["job", id],
    queryFn: async () => {
      const fresh = await api<Job>(`/searches/${id}`);
      // A poll that started before the stream's final event must not turn a
      // finished job back into a running one.
      const known = qc.getQueryData<Job>(["job", id]);
      return known && known.status !== "running" && fresh.status === "running"
        ? known
        : fresh;
    },
    enabled: !!id,
    refetchInterval: (q) =>
      q.state.data?.status === "running" && !streaming ? 3000 : false,
  });
  const running = query.data?.status === "running";
  useEffect(() => {
    if (!id || !running) return;
    const controller = new AbortController();
    let lastID = "";
    async function stream() {
      while (!controller.signal.aborted) {
        try {
          const response = await fetch(`/api/v1/searches/${id}/events`, {
            headers: {
              ...(lastID ? { "Last-Event-ID": lastID } : {}),
            },
            signal: controller.signal,
          });
          if (response.status === 401)
            window.dispatchEvent(new Event("auth-required"));
          if (!response.ok || !response.body) return;
          setStreaming(true);
          const reader = response.body.getReader(),
            decoder = new TextDecoder();
          let buffer = "";
          while (!controller.signal.aborted) {
            const { value, done } = await reader.read();
            if (done) break;
            buffer += decoder.decode(value, { stream: true });
            const parsed = parseEvents(buffer);
            buffer = parsed.rest;
            for (const event of parsed.events) {
              if (event.id) lastID = event.id;
              const job = JSON.parse(event.data) as Job;
              qc.setQueryData(["job", id], job);
              if (job.status !== "running") {
                void qc.invalidateQueries({ queryKey: ["families", id] });
                void qc.invalidateQueries({ queryKey: ["history"] });
                return;
              }
            }
          }
        } catch {
          if (controller.signal.aborted) return;
        } finally {
          setStreaming(false);
        }
        await new Promise((resolve) => setTimeout(resolve, 2000));
      }
    }
    void stream();
    return () => {
      controller.abort();
      setStreaming(false);
    };
  }, [id, running, qc]);
  return query;
}

/** The identifier text that reproduces a saved request in the form. */
function requestIdentifier(media: SearchRequest["media"]) {
  return media.imdbId ?? (media.tmdbId ? `tmdb:${media.tmdbId}` : "");
}

export function SearchPage() {
  const qc = useQueryClient();
  const raven = useRavenReaction();
  const { job: jobID, resolution = "" } = searchRoute.useSearch();
  const navigate = useNavigate();
  const setResolution = (value: string) => {
    void navigate({
      to: "/",
      search: { job: jobID, resolution: value || undefined },
      replace: true,
    });
  };
  const settings = useQuery({
    queryKey: ["settings"],
    queryFn: () => api<Settings>("/settings"),
  });
  const indexers = useQuery({
    queryKey: ["indexers"],
    queryFn: () => api<Indexer[]>("/indexers"),
    enabled: !!settings.data?.hasApiKey,
  });
  const [type, setType] = useState<"movie" | "tv">("movie"),
    [identifier, setIdentifier] = useState(""),
    [season, setSeason] = useState("");
  const [episode, setEpisode] = useState("");
  const preview = useMediaPreview(identifier, type, season);
  const metadataRefresh = useMutation({
    mutationFn: (input: { id: string; type: string; season?: number }) =>
      api("/media/resolve", {
        method: "POST",
        body: JSON.stringify({
          ...input,
          refresh: true,
        }),
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["media-preview"] });
      if (jobID) void qc.invalidateQueries({ queryKey: ["job", jobID] });
    },
  });
  const [selected, setSelected] = useState<number[]>([]),
    [target, setTarget] = useState(0),
    [refresh, setRefresh] = useState(false);
  const [selectedReady, setSelectedReady] = useState(false);
  const [presetsOpen, setPresetsOpen] = useState(false);
  const [presetName, setPresetName] = useState("");
  const [formError, setFormError] = useState("");
  const job = useJob(jobID);
  useEffect(() => {
    if (indexers.data && !selectedReady) {
      setSelected(indexers.data.filter((i) => i.enabled).map((i) => i.id));
      setTarget(settings.data?.defaultTargetIndexer ?? 0);
      setSelectedReady(true);
    }
  }, [indexers.data, settings.data, selectedReady]);
  // Opening a saved search shows what it searched for in the form.
  const prefilled = useRef<string | undefined>(undefined);
  useEffect(() => {
    const data = job.data;
    if (!data || prefilled.current === data.id) return;
    prefilled.current = data.id;
    const media = data.request.media;
    setType(media.type);
    setIdentifier(requestIdentifier(media));
    setSeason(media.season !== undefined ? String(media.season) : "");
    setEpisode(media.episode !== undefined ? String(media.episode) : "");
  }, [job.data]);
  const create = useMutation({
    mutationFn: (request: SearchRequest) =>
      api<Job>("/searches", { method: "POST", body: JSON.stringify(request) }),
    onSuccess: (j) => {
      qc.setQueryData(["job", j.id], j);
      void navigate({
        to: "/",
        search: { job: j.id, resolution: resolution || undefined },
      });
    },
  });
  const rerun = useMutation({
    mutationFn: (request: SearchRequest) =>
      api<Job>("/searches", {
        method: "POST",
        body: JSON.stringify({ ...request, refresh: true }),
      }),
    onSuccess: async (next) => {
      qc.setQueryData(["job", next.id], next);
      await navigate({
        to: "/",
        search: { job: next.id, resolution: resolution || undefined },
      });
    },
  });
  useEffect(() => {
    create.reset();
    rerun.reset();
    // Errors belong to the job they were raised on.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [jobID]);
  function submit(e: FormEvent) {
    e.preventDefault();
    setFormError("");
    const identity = parseMediaInput(identifier);
    if (!identity) {
      setFormError(
        "Enter a valid IMDb ID such as tt0133093, or a positive TMDb ID.",
      );
      return;
    }
    rerun.reset();
    create.mutate({
      media: {
        type,
        ...identity,
        ...(type === "tv" && season !== "" ? { season: +season } : {}),
        ...(type === "tv" && season !== "" && episode !== ""
          ? { episode: +episode }
          : {}),
      },
      indexerIds: selected,
      targetIndexerId: target,
      filters: { minimumCoverage: settings.data?.defaultMinCoverage ?? 2 },
      refresh,
    });
  }
  const busy =
    create.isPending || rerun.isPending || job.data?.status === "running";
  const indexerName = (id: number) =>
    indexers.data?.find((i) => i.id === id)?.name ?? `Indexer ${id}`;
  const enabledCount = indexers.data?.filter((i) => i.enabled).length ?? 0;
  const draftIdentity = parseMediaInput(identifier);
  const draftRequest = {
    type,
    ...draftIdentity,
    ...(type === "tv" && season !== "" ? { season: +season } : {}),
  };
  const jobMedia = job.data?.request.media;
  const draftIsJob =
    !!jobMedia &&
    !!draftIdentity &&
    jobMedia.type === type &&
    (("imdbId" in draftIdentity && draftIdentity.imdbId === jobMedia.imdbId) ||
      ("tmdbId" in draftIdentity && draftIdentity.tmdbId === jobMedia.tmdbId));
  const refreshMetadata = (media: SearchRequest["media"]) =>
    metadataRefresh.isPending
      ? undefined
      : () =>
          metadataRefresh.mutate({
            id: requestIdentifier(media).replace(/^tmdb:/, ""),
            type: media.type,
            ...(media.type === "tv" && media.season !== undefined
              ? { season: media.season }
              : {}),
          });
  const pendingSeeds = create.isPending ? (
    <PendingSeeds names={selected.map(indexerName)} />
  ) : rerun.isPending && job.data ? (
    <PendingSeeds names={job.data.indexers.map((i) => i.name)} />
  ) : null;
  const previewStrip = preview.data?.title && (!jobID || !draftIsJob) && (
    <TitleStrip
      label={jobID ? "Title preview" : "Title"}
      media={preview.data}
      request={draftRequest}
      refresh={draftIdentity ? refreshMetadata(draftRequest) : undefined}
      refreshing={metadataRefresh.isPending}
    >
      {!jobID && pendingSeeds}
    </TitleStrip>
  );
  return (
    <>
      {raven.reaction}
      {settings.data && !settings.data.hasApiKey && (
        <div className="notice info disc-setup">
          <ConnectionIcon size={18} />
          <div>
            <strong>Start with your qui connection.</strong> Connect an existing
            instance to discover your indexers.
          </div>
          <Link to="/settings" className="button primary sm">
            Connect qui <ArrowRightIcon size={14} />
          </Link>
        </div>
      )}
      <section className="panel disc-search" aria-label="Search">
        <form onSubmit={submit} className="disc-search-row">
          <div role="group" aria-label="Media type" className="disc-segment">
            {(["movie", "tv"] as const).map((value) => (
              <button
                key={value}
                type="button"
                aria-pressed={type === value}
                onClick={() => setType(value)}
              >
                {value === "movie" ? "Movie" : "TV"}
              </button>
            ))}
          </div>
          <label className="disc-field-id">
            IMDb or TMDb ID
            <input
              value={identifier}
              onChange={(e) => setIdentifier(e.target.value)}
              placeholder="tt0133093 or tmdb:603"
              required
              spellCheck={false}
              aria-label="Content identifier"
            />
          </label>
          {type === "tv" && (
            <label className="disc-field-sm">
              Season
              {preview.data?.seasons?.length ? (
                <Select
                  aria-label="Season"
                  value={season}
                  onChange={(e) => {
                    setSeason(e.target.value);
                    setEpisode("");
                  }}
                >
                  <option value="">All seasons</option>
                  {preview.data.seasons.map((s) => (
                    <option key={s.number} value={s.number}>
                      {s.name} ({s.episodeCount} episodes)
                    </option>
                  ))}
                </Select>
              ) : (
                <input
                  type="number"
                  min={0}
                  max={999}
                  placeholder="All"
                  value={season}
                  onChange={(e) => {
                    setSeason(e.target.value);
                    if (e.target.value === "") setEpisode("");
                  }}
                />
              )}
            </label>
          )}
          {type === "tv" && (
            <label className="disc-field-xs">
              Episode
              <input
                type="number"
                min={1}
                max={9999}
                placeholder="All"
                disabled={season === ""}
                value={episode}
                onChange={(e) => setEpisode(e.target.value)}
              />
            </label>
          )}
          <label
            className="disc-field-sm"
            title="Filters saved results. Every returned resolution stays available without another search."
          >
            Resolution
            <Select
              aria-label="Resolution"
              value={resolution}
              onChange={(e) => setResolution(e.target.value)}
            >
              <option value="">Any resolution</option>
              {resolution.includes(",") && (
                <option value={resolution}>
                  {resolution.split(",").join(" + ")}
                </option>
              )}
              {resolutions.map((value) => (
                <option key={value} value={value}>
                  {value}
                </option>
              ))}
            </Select>
          </label>
          <div className="disc-field-indexers">
            <span className="disc-field-label" id="disc-indexers-label">
              Indexers
              <button
                type="button"
                className="link-button"
                aria-expanded={presetsOpen}
                aria-controls="disc-presets"
                onClick={() => setPresetsOpen((open) => !open)}
              >
                Presets
              </button>
            </span>
            <Select
              aria-label="Indexers"
              multiple
              value={selected.map(String)}
              triggerLabel={
                <>
                  {selected.length} of {enabledCount}
                  {presetName && (
                    <span className="faint truncate"> · {presetName}</span>
                  )}
                </>
              }
              onChange={(e) => {
                const ids = e.target.selectedOptions.map((o) => +o.value);
                setSelected(ids);
                if (!ids.includes(target)) setTarget(0);
              }}
            >
              {indexers.data?.map((i) => (
                <option key={i.id} value={i.id} disabled={!i.enabled}>
                  <span>
                    {i.name}
                    <small>
                      {i.backend}
                      {!i.enabled && " · disabled"}
                    </small>
                  </span>
                </option>
              ))}
            </Select>
          </div>
          <label className="disc-field-sm">
            Target
            <Select
              aria-label="Target"
              value={target}
              onChange={(e) => setTarget(+e.target.value)}
            >
              <option value={0}>No target</option>
              {indexers.data
                ?.filter((i) => selected.includes(i.id))
                .map((i) => (
                  <option key={i.id} value={i.id}>
                    {i.name}
                  </option>
                ))}
            </Select>
          </label>
          <label
            className="check-row disc-skip-cache"
            title="Ask every indexer again instead of reusing a recent identical search."
          >
            <input
              type="checkbox"
              checked={refresh}
              onChange={(e) => setRefresh(e.target.checked)}
            />
            Skip cache
          </label>
          <Button
            className="primary disc-search-button"
            type="submit"
            disabled={busy || selected.length === 0}
          >
            {busy ? (
              <SpinnerIcon className="spin" size={15} />
            ) : (
              <SearchIcon size={15} />
            )}
            {busy ? "Searching" : "Search"}
          </Button>
        </form>
        <div id="disc-presets" className="disc-presets" hidden={!presetsOpen}>
          <IndexerPresets
            indexers={indexers.data ?? []}
            selected={selected}
            target={target}
            ready={selectedReady}
            onActiveChange={setPresetName}
            apply={(ids, nextTarget) => {
              setSelected(ids);
              setTarget(nextTarget);
            }}
          />
        </div>
        {(formError || create.error || indexers.error) && (
          <div className="disc-search-errors">
            <ErrorNotice error={formError || create.error || indexers.error} />
          </div>
        )}
      </section>
      {preview.isFetching && (
        <p className="hint" role="status">
          Resolving media metadata...
        </p>
      )}
      <ErrorNotice error={preview.error || metadataRefresh.error} />
      {previewStrip}
      {jobID ? (
        <>
          <ErrorNotice error={job.error} />
          {job.data && (
            <>
              <TitleStrip
                media={
                  job.data.media ??
                  (draftIsJob && preview.data?.title ? preview.data : undefined)
                }
                request={job.data.request.media}
                refresh={refreshMetadata(job.data.request.media)}
                refreshing={metadataRefresh.isPending}
              >
                <JobStatus
                  job={job.data}
                  pending={pendingSeeds}
                  refreshing={rerun.isPending}
                  refresh={() => {
                    if (job.data) rerun.mutate(job.data.request);
                  }}
                />
              </TitleStrip>
              <ErrorNotice error={job.data.error || rerun.error} />
              <Results
                key={JSON.stringify(job.data.request)}
                raven={raven}
                job={job.data}
                resolution={resolution}
                setResolution={setResolution}
              />
            </>
          )}
        </>
      ) : (
        <>
          {create.isPending && !previewStrip && (
            <TitleStrip request={draftRequest}>{pendingSeeds}</TitleStrip>
          )}
          {settings.data?.hasApiKey && !previewStrip && !create.isPending && (
            <section className="empty-state disc-empty">
              <p>
                Search a title by IMDb or TMDb ID to compare releases across
                your indexers.
              </p>
              <Link to="/history" className="button sm">
                Recent searches <ArrowRightIcon size={13} />
              </Link>
            </section>
          )}
        </>
      )}
    </>
  );
}
