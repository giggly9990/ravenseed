import { useRef, useState, type KeyboardEvent, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import type {
  Indexer,
  IndexerPolicy,
  Monitor,
  PolicyPreset,
  SlotAxes,
} from "../types";
import { ErrorNotice, policyResolutions } from "../app/shared";
import { Button } from "./ui/button";
import { Select } from "./ui/select";
import { Dialog } from "./ui/dialog";
import { ChevronRightIcon, CloseIcon, SpinnerIcon } from "./icons";
import {
  ChipInput,
  SaveBar,
  SavedNotice,
  useReportDirty,
} from "./settings-workspace";
import { countChanges } from "../hooks/use-settings-draft";
import { useCandidateCounts } from "../hooks/use-candidate-counts";
import { relativeTime } from "../lib/time";

const classes = [
  "FULL_DISC",
  "REMUX",
  "ENCODE",
  "WEB_DL",
  "WEBRIP",
  "HDTV",
  "OTHER",
];
const classLabels: Record<string, string> = {
  FULL_DISC: "Full disc",
  REMUX: "Remux",
  ENCODE: "Encode",
  WEB_DL: "WEB-DL",
  WEBRIP: "WEBRip",
  HDTV: "HDTV",
  OTHER: "Other",
};
const axes: { key: keyof SlotAxes; label: string; title: string }[] = [
  { key: "releaseClass", label: "Class", title: "Release class" },
  { key: "resolution", label: "Resolution", title: "Resolution" },
  { key: "source", label: "Source", title: "Source" },
  { key: "provider", label: "Provider", title: "WEB provider" },
  { key: "codec", label: "Codec", title: "Video codec" },
  { key: "hdr", label: "HDR", title: "HDR / colour space" },
  { key: "cut", label: "Cut", title: "Cut / edition" },
  { key: "tvStructure", label: "TV pack", title: "TV structure" },
  { key: "group", label: "Group", title: "Release group" },
  { key: "hybrid", label: "Hybrid", title: "Hybrid / native" },
];
const flags: { key: keyof IndexerPolicy; name: string; desc: string }[] = [
  {
    key: "detectMissing",
    name: "Missing slots",
    desc: "Target has nothing in this slot.",
  },
  {
    key: "detectCoexistence",
    name: "Coexistence",
    desc: "Slot is taken but rules allow a second.",
  },
  {
    key: "detectTrump",
    name: "Trumps",
    desc: "Objectively better than what the target has.",
  },
  {
    key: "detectReview",
    name: "Needs review",
    desc: "Could be better; a person should decide.",
  },
];
const corrections = ["REPACK", "PROPER", "RERIP", "REISSUE"] as const;

type Draft = { base: IndexerPolicy; value: IndexerPolicy };

/** Policy fields only, for comparing a policy with its preset. */
function rules(policy: IndexerPolicy) {
  const value: Partial<IndexerPolicy> = { ...policy };
  delete value.indexerId;
  delete value.snapshotId;
  delete value.updatedAt;
  delete value.presetId;
  return value;
}

function presetEdited(policy: IndexerPolicy, presets?: PolicyPreset[]) {
  const preset = presets?.find((p) => p.id === policy.presetId);
  return !!preset && countChanges(rules(policy), rules(preset.policy)) > 0;
}

export function IndexerPolicies({ ready }: { ready: boolean }) {
  const [chosen, setChosen] = useState(0);
  const [drafts, setDrafts] = useState<Record<number, Draft>>({});
  const indexers = useQuery({
    queryKey: ["indexers"],
    queryFn: () => api<Indexer[]>("/indexers"),
    enabled: ready,
  });
  const savedPolicies = useQuery({
    queryKey: ["indexer-policies"],
    queryFn: () => api<IndexerPolicy[]>("/indexer-policies"),
    enabled: ready,
  });
  const presets = useQuery({
    queryKey: ["policy-presets"],
    queryFn: () => api<PolicyPreset[]>("/policy-presets"),
    enabled: ready,
  });
  const enabled = indexers.data?.filter((i) => i.enabled) ?? [];
  const selected = enabled.some((i) => i.id === chosen)
    ? chosen
    : (enabled[0]?.id ?? 0);
  const total = Object.values(drafts).reduce(
    (sum, d) => sum + countChanges(d.value, d.base),
    0,
  );
  useReportDirty(total);
  const options = useRef<(HTMLButtonElement | null)[]>([]);
  const onKeyDown = (event: KeyboardEvent, index: number) => {
    let next = index;
    if (event.key === "ArrowDown")
      next = Math.min(index + 1, enabled.length - 1);
    else if (event.key === "ArrowUp") next = Math.max(index - 1, 0);
    else if (event.key === "Home") next = 0;
    else if (event.key === "End") next = enabled.length - 1;
    else return;
    event.preventDefault();
    setChosen(enabled[next].id);
    options.current[next]?.focus();
  };
  const subLabel = (id: number) => {
    if (drafts[id]) return { text: "Draft · not saved", state: "draft" };
    const policy = savedPolicies.data?.find((p) => p.indexerId === id);
    if (!policy) return { text: "No policy", state: "off" };
    const preset = presets.data?.find((p) => p.id === policy.presetId);
    const name = preset
      ? `${preset.name}${presetEdited(policy, presets.data) ? " · edited" : ""}`
      : "Custom";
    return policy.enabled
      ? { text: name, state: "on" }
      : { text: `${name} · off`, state: "off" };
  };
  return (
    <section className="set-section" aria-labelledby="set-policies-title">
      <header className="set-head">
        <h2 id="set-policies-title">Indexer policies</h2>
        <span className="hint">
          What counts as an upload candidate on each target. Upload-Assistant
          still checks the actual upload.
        </span>
      </header>
      {!ready && (
        <p className="panel table-empty">
          Connect qui to choose a target indexer.
        </p>
      )}
      <ErrorNotice error={indexers.error ?? savedPolicies.error} />
      {ready && (
        <div className="pol-layout">
          <div
            className="panel pol-targets"
            role="listbox"
            aria-label="Target indexer"
            aria-orientation="vertical"
          >
            {indexers.isPending && <p className="table-empty">Loading…</p>}
            {enabled.map((indexer, index) => {
              const sub = subLabel(indexer.id);
              const isSelected = indexer.id === selected;
              return (
                <button
                  key={indexer.id}
                  ref={(node) => {
                    options.current[index] = node;
                  }}
                  type="button"
                  role="option"
                  aria-selected={isSelected}
                  tabIndex={isSelected ? 0 : -1}
                  className="pol-target"
                  onClick={() => setChosen(indexer.id)}
                  onKeyDown={(event) => onKeyDown(event, index)}
                >
                  <span className="pol-target-text">
                    <strong>{indexer.name}</strong>
                    <small>{sub.text}</small>
                  </span>
                  <span className={`pol-dot ${sub.state}`} aria-hidden="true" />
                </button>
              );
            })}
          </div>
          {selected > 0 && (
            <PolicyEditor
              key={selected}
              indexer={enabled.find((i) => i.id === selected)!}
              presets={presets.data}
              draft={drafts[selected]}
              setDraft={(next) =>
                setDrafts((all) => {
                  const copy = { ...all };
                  if (next && countChanges(next.value, next.base) > 0)
                    copy[selected] = next;
                  else delete copy[selected];
                  return copy;
                })
              }
            />
          )}
        </div>
      )}
    </section>
  );
}

function PolicyEditor({
  indexer,
  presets,
  draft,
  setDraft,
}: {
  indexer: Indexer;
  presets?: PolicyPreset[];
  draft?: Draft;
  setDraft: (draft: Draft | undefined) => void;
}) {
  const query = useQuery({
    queryKey: ["indexer-policy", indexer.id],
    queryFn: () => api<IndexerPolicy>(`/indexer-policies/${indexer.id}`),
  });
  if (query.isPending)
    return <p className="panel table-empty pol-editor">Loading policy…</p>;
  if (query.error || !query.data)
    return (
      <div className="pol-editor">
        <ErrorNotice error={query.error} />
      </div>
    );
  return (
    <PolicyForm
      indexer={indexer}
      saved={query.data}
      presets={presets}
      draft={draft}
      setDraft={setDraft}
    />
  );
}

function PolicyForm({
  indexer,
  saved,
  presets,
  draft,
  setDraft,
}: {
  indexer: Indexer;
  saved: IndexerPolicy;
  presets?: PolicyPreset[];
  draft?: Draft;
  setDraft: (draft: Draft | undefined) => void;
}) {
  const qc = useQueryClient();
  const base = draft?.base ?? saved;
  const form = draft?.value ?? saved;
  const changes = draft ? countChanges(draft.value, draft.base) : 0;
  const [justSaved, setJustSaved] = useState(false);
  const [jsonOpen, setJsonOpen] = useState(false);
  const monitors = useQuery({
    queryKey: ["monitors"],
    queryFn: () => api<Monitor[]>("/monitors"),
  });
  const targeting = monitors.data?.filter(
    (m) => m.targetIndexerId === form.indexerId,
  );
  const counts = useCandidateCounts(targeting, ["all"], 60_000);
  const candidateTotal = targeting?.length
    ? targeting.reduce<number | undefined>(
        (sum, m) =>
          sum === undefined || counts[m.id]?.all === undefined
            ? undefined
            : sum + counts[m.id].all!,
        0,
      )
    : undefined;
  const save = useMutation({
    mutationFn: () =>
      api<IndexerPolicy>(`/indexer-policies/${form.indexerId}`, {
        method: "PUT",
        body: JSON.stringify(form),
      }),
    onSuccess: (data) => {
      qc.setQueryData(["indexer-policy", data.indexerId], data);
      void qc.invalidateQueries({ queryKey: ["indexer-policies"] });
      setDraft(undefined);
      setJustSaved(true);
    },
  });
  const recheck = useMutation({
    mutationFn: () =>
      api<{ queued: number }>(`/indexer-policies/${form.indexerId}/recheck`, {
        method: "POST",
        body: "{}",
      }),
  });
  const replace = (value: IndexerPolicy) => {
    setJustSaved(false);
    setDraft({ base, value });
  };
  function update<K extends keyof IndexerPolicy>(
    key: K,
    value: IndexerPolicy[K],
  ) {
    replace({ ...form, [key]: value });
  }
  const check = (key: keyof IndexerPolicy, label: ReactNode) => (
    <label className="check-row">
      <input
        type="checkbox"
        checked={Boolean(form[key])}
        onChange={(e) => update(key, e.target.checked as never)}
      />
      {label}
    </label>
  );
  const preset = presets?.find((item) => item.id === form.presetId);
  const edited = presetEdited(form, presets);
  return (
    <form
      className="pol-editor set-form"
      onSubmit={(event) => {
        event.preventDefault();
        save.mutate();
      }}
    >
      <div className="panel pol-bar">
        <strong className="pol-bar-name">{indexer.name}</strong>
        <span className="pill quiet">
          {preset
            ? `Based on ${preset.name}${edited ? " · edited" : ""}`
            : base.snapshotId
              ? "Custom policy"
              : "No policy saved"}
        </span>
        {base.updatedAt && (
          <span className="hint">Saved {relativeTime(base.updatedAt)}</span>
        )}
        <div className="pol-bar-actions">
          <Button
            type="button"
            className="sm quiet"
            onClick={() => setJsonOpen(true)}
          >
            Import / export JSON
          </Button>
          {base.snapshotId && (
            <Button
              type="button"
              className="sm"
              disabled={recheck.isPending || changes > 0}
              title={changes > 0 ? "Save the policy first" : undefined}
              onClick={() => recheck.mutate()}
            >
              {recheck.isPending && <SpinnerIcon size={14} className="spin" />}
              {candidateTotal !== undefined
                ? `Recheck ${candidateTotal} candidates`
                : "Recheck candidates"}
            </Button>
          )}
          <Button
            className="sm primary"
            type="submit"
            disabled={save.isPending}
          >
            {save.isPending && <SpinnerIcon size={14} className="spin" />}
            Save policy
          </Button>
        </div>
        <div className="pol-bar-row">
          <label className="switch-row">
            <button
              type="button"
              role="switch"
              className="switch"
              aria-checked={form.enabled}
              aria-label="Enable candidate policy"
              onClick={() => update("enabled", !form.enabled)}
            />
            Policy active
          </label>
          <label className="inline-label pol-preset">
            Start from a ruleset
            <span className="select-compact">
              <Select
                aria-label="Policy preset"
                value={form.presetId ?? ""}
                onChange={(e) => {
                  const next = presets?.find(
                    (item) => item.id === e.target.value,
                  );
                  if (next)
                    replace({
                      ...next.policy,
                      indexerId: form.indexerId,
                      snapshotId: undefined,
                      updatedAt: undefined,
                    });
                  // Keeps the rules and drops the ruleset label.
                  else update("presetId", undefined);
                }}
              >
                <option value="">Custom policy</option>
                {presets?.map((item) => (
                  <option key={item.id} value={item.id}>
                    {item.name}
                  </option>
                ))}
              </Select>
            </span>
          </label>
          {preset && (
            <span className="hint pol-preset-desc">{preset.description}</span>
          )}
        </div>
      </div>

      {justSaved && (
        <SavedNotice>
          Policy saved. New evaluations will use this version.
        </SavedNotice>
      )}
      {recheck.data && (
        <p role="status" className="notice info">
          Queued {recheck.data.queued} monitor candidates for fresh target
          checks.
        </p>
      )}
      <ErrorNotice error={save.error ?? recheck.error} />

      <fieldset className="pol-flags">
        <legend className="visually-hidden">What to flag</legend>
        {flags.map((flag) => (
          <label key={flag.key} className="panel pol-flag">
            <span className="check-row">
              <input
                type="checkbox"
                checked={Boolean(form[flag.key])}
                onChange={(e) => update(flag.key, e.target.checked as never)}
              />
              <strong>{flag.name}</strong>
            </span>
            <span className="hint">{flag.desc}</span>
          </label>
        ))}
      </fieldset>

      <div className="panel">
        <div className="panel-head">
          <h3 id={`pol-slots-${form.indexerId}`}>Slots</h3>
          <span className="hint">
            Ticked traits make a separate slot. A missing provider or codec
            leaves the slot unevaluated; unmarked HDR is never assumed SDR.
          </span>
        </div>
        <div className="table-wrap">
          <table
            className="data-table compact pol-matrix"
            aria-labelledby={`pol-slots-${form.indexerId}`}
          >
            <thead>
              <tr>
                <th scope="col">
                  <span className="visually-hidden">Release class</span>
                </th>
                {axes.map((axis) => (
                  <th key={axis.key} scope="col" title={axis.title}>
                    {axis.label}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {classes.map((releaseClass) => {
                const rule = form.slots[releaseClass];
                if (!rule) return null;
                const name = classLabels[releaseClass] ?? releaseClass;
                return (
                  <tr key={releaseClass}>
                    <th scope="row">{name}</th>
                    {axes.map((axis) => (
                      <td key={axis.key}>
                        <input
                          type="checkbox"
                          aria-label={`${name}: ${axis.title.toLowerCase()}`}
                          checked={rule[axis.key]}
                          onChange={(e) =>
                            update("slots", {
                              ...form.slots,
                              [releaseClass]: {
                                ...rule,
                                [axis.key]: e.target.checked,
                              },
                            })
                          }
                        />
                      </td>
                    ))}
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
        <div className="panel-foot">
          <label className="inline-label pol-number">
            Separate encode slots when sizes differ by more than
            <input
              type="number"
              min="0"
              max="100"
              step="0.1"
              aria-label="Encode size difference for separate slots (%)"
              value={form.encodeSizePercent}
              onChange={(e) => update("encodeSizePercent", +e.target.value)}
            />
            %
          </label>
          <span className="hint">
            0 turns it off. Size defines the slot; it never proves better video.
          </span>
        </div>
        {form.encodeSizePercent > 0 && (
          <div className="panel-foot pol-size-scope">
            <div className="stack pol-tight">
              <span className="hint">
                Only for these encodes (resolution:codec); empty means all
              </span>
              <ChipInput
                label="Add encode size scope"
                placeholder="1080p:AVC"
                values={form.encodeSizeScope ?? []}
                transform={encodeScope}
                invalidHint="Use resolution:codec, for example 1080p:AVC."
                onChange={(next) => update("encodeSizeScope", next)}
              />
            </div>
            <label className="inline-label pol-number">
              At most
              <input
                type="number"
                min="0"
                max="10"
                aria-label="Maximum size-separated encode slots"
                value={form.encodeSizeSlots ?? 0}
                onChange={(e) => update("encodeSizeSlots", +e.target.value)}
              />
              size slots (0 is unlimited)
            </label>
          </div>
        )}
        <div className="panel-foot">
          <BitrateTiers
            value={form.encodeBitrateTiers ?? []}
            onChange={(next) => update("encodeBitrateTiers", next)}
          />
        </div>
      </div>

      <div className="pol-boxes">
        <div className="panel pad stack pol-box">
          <h3>Trump rules</h3>
          <div className="pol-corrections">
            <span className="hint">Beats the original</span>
            <div className="chips">
              {corrections.map((name) => {
                const on = form.corrections.includes(name);
                return (
                  <button
                    key={name}
                    type="button"
                    className="chip"
                    aria-pressed={on}
                    onClick={() =>
                      update(
                        "corrections",
                        on
                          ? form.corrections.filter((item) => item !== name)
                          : [...form.corrections, name],
                      )
                    }
                  >
                    {name}
                  </button>
                );
              })}
            </div>
          </div>
          {check("webDlOverWebRip", "WEB-DL beats same-service WEBRip")}
          {check("groupOverNoGrp", "Reputable group beats NoGrp")}
          {check("seasonPackOverEpisodes", "Season pack beats single episodes")}
          {check(
            "episodesBlockedByPack",
            "Any pack of a season blocks its single episodes",
          )}
          <SourcePriority
            value={form.sourcePriority}
            onChange={(next) => update("sourcePriority", next)}
          />
          <HDRRelations
            value={form.hdrRelations}
            onChange={(next) => update("hdrRelations", next)}
          />
        </div>
        <div className="panel pad stack pol-box">
          <h3>Groups and review</h3>
          <div className="stack pol-tight">
            <span className="hint">Reputable groups</span>
            <ChipInput
              label="Add reputable group"
              placeholder="Add group…"
              values={form.reputableGroups}
              transform={(v) => v.toUpperCase()}
              onChange={(next) => update("reputableGroups", next)}
            />
          </div>
          <div className="stack pol-tight">
            <span className="hint">
              Banned groups on this tracker (never offered; an occupant from one
              can be trumped)
            </span>
            <ChipInput
              label="Add banned group"
              placeholder="Add group…"
              values={form.bannedGroups ?? []}
              transform={(v) => v.toUpperCase()}
              onChange={(next) => update("bannedGroups", next)}
            />
          </div>
          <div className="stack pol-tight">
            <span className="hint">
              Internal groups on this tracker (internal status only counts here)
            </span>
            <ChipInput
              label="Add internal group"
              placeholder="Add group…"
              values={form.internalGroups ?? []}
              transform={(v) => v.toUpperCase()}
              onChange={(next) => update("internalGroups", next)}
            />
            <Select
              aria-label="Internal releases"
              value={form.internalSlots ?? ""}
              onChange={(e) =>
                update(
                  "internalSlots",
                  e.target.value as IndexerPolicy["internalSlots"],
                )
              }
            >
              <option value="">Internals are not evaluated</option>
              <option value="separate">
                Internals have their own slot beside non-internal releases
              </option>
              <option value="review">
                An internal may take an extra slot or trump: review
              </option>
            </Select>
          </div>
          <span className="hint">Send to Needs review when</span>
          {check(
            "reviewVisualQuality",
            "Visual quality can't be compared from the name",
          )}
          {check(
            "reviewHDRComparability",
            "An HDR trump needs a manual quality comparison",
          )}
          {check("reviewSuperiorMaster", "Source may be a superior master")}
          {check(
            "reviewDualAudio",
            "A dual-audio release meets a single-language one",
          )}
          {check(
            "reviewLosslessWeb",
            "A 2160p WEB-DL with lossless audio may take the extra slot",
          )}
          <label
            className="inline-label pol-number"
            title="Seeder-based trumping needs reliable availability evidence and isn't supported by the server yet."
          >
            Treat as dead at
            <input
              type="number"
              min={0}
              aria-label="Dead torrent seeders"
              placeholder="off"
              value={form.deadTorrentMaxSeeders ?? ""}
              onChange={(e) =>
                update(
                  "deadTorrentMaxSeeders",
                  e.target.value === "" ? undefined : +e.target.value,
                )
              }
            />
            seeders or fewer
          </label>
          <span className="hint">
            Leave blank. The server doesn't support seeder-based trumps yet.
          </span>
        </div>
      </div>

      <SaveBar
        report={false}
        label={`${indexer.name} policy`}
        changes={changes}
        saving={save.isPending}
        onDiscard={() => setDraft(undefined)}
      />
      <PolicyJSON
        open={jsonOpen}
        onOpenChange={setJsonOpen}
        form={form}
        onImport={(value) => {
          replace({
            ...value,
            indexerId: form.indexerId,
            snapshotId: undefined,
            updatedAt: undefined,
          });
          setJsonOpen(false);
        }}
      />
    </form>
  );
}

function SourcePriority({
  value,
  onChange,
}: {
  value: string[];
  onChange: (value: string[]) => void;
}) {
  const [text, setText] = useState("");
  const move = (from: number, to: number) => {
    const next = [...value];
    const [item] = next.splice(from, 1);
    next.splice(to, 0, item);
    onChange(next);
  };
  const add = () => {
    const item = text.trim().toUpperCase();
    if (item && !value.includes(item)) onChange([...value, item]);
    setText("");
  };
  return (
    <div className="stack pol-tight">
      <span className="hint" id="pol-source-hint">
        Source priority, low to high. Compares same class, resolution, codec and
        HDR.
      </span>
      {value.length > 0 && (
        <ol className="pol-order" aria-describedby="pol-source-hint">
          {value.map((source, i) => (
            <li key={source} className="pol-order-item">
              <span className="mono">{source}</span>
              <button
                type="button"
                className="icon-button"
                aria-label={`Move ${source} lower`}
                disabled={i === 0}
                onClick={() => move(i, i - 1)}
              >
                <ChevronRightIcon size={13} className="pol-flip" />
              </button>
              <button
                type="button"
                className="icon-button"
                aria-label={`Move ${source} higher`}
                disabled={i === value.length - 1}
                onClick={() => move(i, i + 1)}
              >
                <ChevronRightIcon size={13} />
              </button>
              <button
                type="button"
                className="icon-button"
                aria-label={`Remove ${source}`}
                onClick={() => onChange(value.filter((v) => v !== source))}
              >
                <CloseIcon size={12} />
              </button>
            </li>
          ))}
        </ol>
      )}
      <div className="pol-add-row">
        <input
          className="mono"
          aria-label="Add source"
          placeholder="e.g. WEB_DL"
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              add();
            }
          }}
        />
        <Button type="button" className="sm quiet" onClick={add}>
          Add
        </Button>
      </div>
    </div>
  );
}

const tierResolutions = policyResolutions.filter((r) =>
  ["720p", "1080p", "1440p", "2160p"].includes(r),
);

/** "1080P:avc" → "1080p:AVC"; "" for anything the server would reject. */
export function encodeScope(value: string) {
  const [resolution, codec = "", ...rest] = value.split(":");
  const r = resolution.trim().toLowerCase();
  const c = codec.trim().toUpperCase();
  return !rest.length && policyResolutions.includes(r) && c && c.length <= 10
    ? `${r}:${c}`
    : "";
}

/** Compact/transparent encode tiers by estimated video bitrate per resolution. */
function BitrateTiers({
  value,
  onChange,
}: {
  value: { resolution: string; maxVideoMbps: number }[];
  onChange: (value: { resolution: string; maxVideoMbps: number }[]) => void;
}) {
  const free = tierResolutions.filter(
    (r) => !value.some((tier) => tier.resolution === r),
  );
  return (
    <div className="stack pol-tight">
      <span className="hint">
        Encode size tiers: compact up to this video bitrate, transparent above.
        Uses the runtime from Sonarr, Radarr or TMDb; without it the finding
        goes to review.
      </span>
      {value.length > 0 && (
        <ul className="pol-relations">
          {value.map((tier, i) => (
            <li key={tier.resolution}>
              <span className="mono">{tier.resolution}</span>
              <label className="inline-label pol-number">
                up to
                <input
                  type="number"
                  min="1"
                  max="200"
                  step="0.5"
                  aria-label={`${tier.resolution} compact tier limit (Mbps)`}
                  value={tier.maxVideoMbps}
                  onChange={(e) =>
                    onChange(
                      value.map((v, n) =>
                        n === i ? { ...v, maxVideoMbps: +e.target.value } : v,
                      ),
                    )
                  }
                />
                Mbps
              </label>
              <button
                type="button"
                className="icon-button"
                aria-label={`Remove ${tier.resolution} tier`}
                onClick={() => onChange(value.filter((_, n) => n !== i))}
              >
                <CloseIcon size={12} />
              </button>
            </li>
          ))}
        </ul>
      )}
      {free.length > 0 && (
        <Select
          aria-label="Add encode size tier"
          value=""
          onChange={(e) =>
            e.target.value &&
            onChange([
              ...value,
              { resolution: e.target.value, maxVideoMbps: 12 },
            ])
          }
        >
          <option value="">Add a size tier…</option>
          {free.map((r) => (
            <option key={r} value={r}>
              {r}
            </option>
          ))}
        </Select>
      )}
    </div>
  );
}

function HDRRelations({
  value,
  onChange,
}: {
  value: { from: string; to: string }[];
  onChange: (value: { from: string; to: string }[]) => void;
}) {
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const add = () => {
    const pair = {
      from: from.trim().toUpperCase(),
      to: to.trim().toUpperCase(),
    };
    if (!pair.from || !pair.to) return;
    onChange([...value, pair]);
    setFrom("");
    setTo("");
  };
  return (
    <div className="stack pol-tight">
      <span className="hint">
        HDR upgrades (candidate beats target). No global ranking is assumed.
      </span>
      {value.length > 0 && (
        <ul className="pol-relations">
          {value.map((r, i) => (
            <li key={`${r.from}>${r.to}:${i}`}>
              <span className="mono">
                {r.from} → {r.to}
              </span>
              <button
                type="button"
                className="icon-button"
                aria-label={`Remove ${r.from} over ${r.to}`}
                onClick={() => onChange(value.filter((_, n) => n !== i))}
              >
                <CloseIcon size={12} />
              </button>
            </li>
          ))}
        </ul>
      )}
      <div className="pol-add-row">
        <input
          className="mono"
          aria-label="HDR candidate"
          placeholder="DV+HDR10"
          value={from}
          onChange={(e) => setFrom(e.target.value)}
        />
        <span className="faint" aria-hidden="true">
          →
        </span>
        <input
          className="mono"
          aria-label="HDR target"
          placeholder="HDR10"
          value={to}
          onChange={(e) => setTo(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              add();
            }
          }}
        />
        <Button
          type="button"
          className="sm quiet"
          disabled={!from.trim() || !to.trim()}
          onClick={add}
        >
          Add
        </Button>
      </div>
    </div>
  );
}

/**
 * Every IndexerPolicy field the server accepts on PUT. The server rejects
 * unknown fields, so imports keep only these (the export adds `version`).
 * Typed as a Record so adding a policy field without listing it fails tsc.
 */
const policyFields: Record<
  Exclude<keyof IndexerPolicy, "snapshotId" | "updatedAt">,
  true
> = {
  indexerId: true,
  enabled: true,
  detectMissing: true,
  detectCoexistence: true,
  detectTrump: true,
  detectReview: true,
  slots: true,
  encodeSizePercent: true,
  encodeSizeScope: true,
  encodeSizeSlots: true,
  encodeBitrateTiers: true,
  episodesBlockedByPack: true,
  reviewDualAudio: true,
  reviewLosslessWeb: true,
  bannedGroups: true,
  internalGroups: true,
  internalSlots: true,
  corrections: true,
  webDlOverWebRip: true,
  sourcePriority: true,
  hdrRelations: true,
  groupOverNoGrp: true,
  reputableGroups: true,
  seasonPackOverEpisodes: true,
  deadTorrentMaxSeeders: true,
  reviewVisualQuality: true,
  reviewHDRComparability: true,
  reviewSuperiorMaster: true,
  presetId: true,
};

function pick<T extends object>(value: T, keys: readonly string[]): T {
  if (!value || typeof value !== "object") return value;
  return Object.fromEntries(
    Object.entries(value).filter(([key]) => keys.includes(key)),
  ) as T;
}

/** Strips fields the server would reject from pasted policy JSON. */
export function importablePolicy(value: IndexerPolicy): IndexerPolicy {
  const policy = pick(value, Object.keys(policyFields));
  const axisKeys = axes.map((a) => a.key);
  policy.slots = Object.fromEntries(
    classes.map((name) => [name, pick(value.slots[name], axisKeys)]),
  );
  policy.hdrRelations = value.hdrRelations.map((r) => pick(r, ["from", "to"]));
  if (Array.isArray(value.encodeBitrateTiers))
    policy.encodeBitrateTiers = value.encodeBitrateTiers.map((t) =>
      pick(t, ["resolution", "maxVideoMbps"]),
    );
  return policy;
}

function PolicyJSON({
  open,
  onOpenChange,
  form,
  onImport,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  form: IndexerPolicy;
  onImport: (value: IndexerPolicy) => void;
}) {
  const [importText, setImportText] = useState("");
  const [importError, setImportError] = useState("");
  const exportValue = JSON.stringify(
    { ...form, version: 1, snapshotId: undefined, updatedAt: undefined },
    null,
    2,
  );
  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title="Import / export policy JSON"
      description="Copy this draft, or paste a policy to replace it. Nothing is saved until you press Save."
      wide
    >
      <div className="dialog-body pol-json">
        <label>
          Current draft JSON
          <textarea readOnly rows={12} value={exportValue} />
        </label>
        <label>
          Paste policy JSON to import
          <textarea
            rows={12}
            value={importText}
            onChange={(e) => setImportText(e.target.value)}
          />
        </label>
      </div>
      {importError && (
        <p role="alert" className="notice error">
          {importError}
        </p>
      )}
      <div className="dialog-actions">
        <Button
          type="button"
          className="primary"
          disabled={!importText.trim()}
          onClick={() => {
            try {
              const value = JSON.parse(importText) as IndexerPolicy;
              if (
                !value ||
                typeof value !== "object" ||
                !value.slots ||
                !Array.isArray(value.corrections) ||
                !Array.isArray(value.sourcePriority) ||
                !Array.isArray(value.hdrRelations) ||
                !Array.isArray(value.reputableGroups) ||
                classes.some((name) => !value.slots[name])
              )
                throw new Error(
                  "Policy JSON needs complete slot and rule arrays",
                );
              setImportError("");
              setImportText("");
              onImport(importablePolicy(value));
            } catch (error) {
              setImportError(
                error instanceof Error ? error.message : "Invalid policy JSON",
              );
            }
          }}
        >
          Apply imported JSON to draft
        </Button>
      </div>
    </Dialog>
  );
}
