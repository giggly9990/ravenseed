import { Fragment, useState } from "react";
import { ChevronDownIcon, ChevronRightIcon } from "../../components/icons";
import { bytes, safeURL } from "../../api";
import type { Family, Result } from "../../types";
import { Button } from "../../components/ui/button";
import { TrackerFlags } from "../../components/tracker-flags";
import { PolicySummary, policyLabel } from "../../components/policy-summary";
import { AddTorrent } from "../../components/add-torrent";
import { label } from "../../app/shared";

type Tone = "ok" | "accent" | "warn" | "quiet";

const nice: Record<string, string> = {
  UHD_BLURAY: "UHD BluRay",
  BLURAY: "BluRay",
  WEB_DL: "WEB-DL",
  WEBRIP: "WEBRip",
  FULL_DISC: "Full disc",
  REMUX: "Remux",
  ENCODE: "Encode",
  OTHER: "Other",
};
/** Display form of a parser token such as UHD_BLURAY or WEB_DL. */
export const releaseLabel = (token: string) => nice[token] ?? label(token);

function releaseKind(p: Family["canonical"]) {
  const web = ["WEB", "WEB_DL"].includes(p.source);
  if (web && ["WEB_DL", "WEBRIP"].includes(p.releaseType))
    return releaseLabel(p.releaseType);
  return [p.source, p.releaseType]
    .filter((t) => t && t !== "UNKNOWN")
    .map(releaseLabel)
    .join(" ");
}

/** The family's standing on the target, as a pill tone and short label. */
export function targetPill(f: Family): { tone: Tone; text: string } {
  const t = f.targetStatus;
  const policy = f.policy ?? f.variants.find((v) => v.policy)?.policy;
  const cls = policy?.classification;
  if (!t.evaluated || cls === "unevaluated")
    return { tone: "quiet", text: "Not evaluated" };
  if (cls === "equivalent" || t.exactMatchFound)
    return { tone: "ok", text: "On target" };
  if (cls === "trump" || cls === "possible_trump" || cls === "missing_slot")
    return { tone: "accent", text: policyLabel(cls) };
  if (cls === "coexistence") return { tone: "warn", text: "Coexist" };
  if (cls === "not_eligible") return { tone: "quiet", text: "Not eligible" };
  if (t.candidate) return { tone: "accent", text: "Candidate" };
  return {
    tone: "quiet",
    text: t.relatedMatchFound ? "Related only" : "Not on target",
  };
}

function tvInfo(p: Family["canonical"]) {
  if (p.mediaType !== "tv") return "";
  return p.episodes?.length
    ? `S${p.season} E${p.episodes.join(", E")}`
    : p.seasons?.length
      ? `Season pack ${p.seasons.join(", ")}`
      : p.absoluteEpisodes?.length
        ? `Absolute ${p.absoluteEpisodes.join(", ")}`
        : p.structure;
}

function specLine(p: Family["canonical"]) {
  return [
    p.service,
    p.videoCodec,
    ...p.hdr,
    p.audioCodec && `${p.audioCodec} ${p.audioChannels}`.trim(),
    p.hybrid && "Hybrid",
    p.correction &&
      `${p.correction}${(p.revision ?? 1) > 1 ? ` ${p.revision}` : ""}`,
    ...(p.edition ?? []),
    tvInfo(p),
  ]
    .filter(Boolean)
    .join(" · ");
}

export function FamilyTable({
  jobId,
  families,
  selected,
  targetName,
  onDetails,
}: {
  jobId: string;
  families: Family[];
  selected: number;
  targetName?: string;
  onDetails: (family: Family) => void;
}) {
  const [open, setOpen] = useState<Set<string>>(() => new Set());
  const toggle = (id: string) =>
    setOpen((current) => {
      const next = new Set(current);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  return (
    <div className="panel table-wrap disc-table-wrap">
      <table className="data-table disc-families">
        <caption className="visually-hidden">Release families</caption>
        <thead>
          <tr>
            <th scope="col">Coverage</th>
            <th scope="col">Release</th>
            <th scope="col" className="num">
              Size
            </th>
            <th scope="col" className="num">
              Seeds
            </th>
            <th scope="col">On indexers</th>
            <th scope="col">{targetName ?? "Target"}</th>
            <th scope="col">
              <span className="visually-hidden">Details</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {families.map((f) => (
            <FamilyRow
              key={f.id}
              jobId={jobId}
              family={f}
              selected={selected}
              targetName={targetName}
              expanded={open.has(f.id)}
              toggle={() => toggle(f.id)}
              onDetails={() => onDetails(f)}
            />
          ))}
        </tbody>
      </table>
    </div>
  );
}

function FamilyRow({
  jobId,
  family: f,
  selected,
  targetName,
  expanded,
  toggle,
  onDetails,
}: {
  jobId: string;
  family: Family;
  selected: number;
  targetName?: string;
  expanded: boolean;
  toggle: () => void;
  onDetails: () => void;
}) {
  const p = f.canonical;
  const trackers = [...new Set(f.results.map((r) => r.indexerId))];
  const pill = targetPill(f);
  const share = selected ? Math.min(1, f.coverage / selected) : 0;
  const spec = specLine(p);
  const kind = releaseKind(p);
  const detailsId = `disc-family-${f.id}`;
  return (
    <>
      <tr className={expanded ? "selected disc-open" : undefined}>
        <td className="disc-cov">
          <span className="disc-cov-n">
            <strong>{f.coverage}</strong>
            <span className="faint">/{selected}</span>
          </span>
          <span
            className={`bar${pill.tone === "ok" ? " ok" : ""}`}
            aria-hidden="true"
          >
            <span style={{ width: `${Math.round(share * 100)}%` }} />
          </span>
        </td>
        <td className="disc-release">
          <div className="disc-release-line">
            <span className="res-tag">{p.resolution || "?"}</span>
            <strong>{kind || f.results[0]?.title}</strong>
            <span className="mono disc-group">
              {p.displayGroup || p.group || "NOGRP"}
            </span>
            {f.confidence !== "EXACT" && (
              <span
                className="pill quiet"
                title="Match confidence for this family"
              >
                {f.confidence.toLowerCase()}
              </span>
            )}
          </div>
          {spec && (
            <span className="disc-spec truncate" title={spec}>
              {spec}
            </span>
          )}
        </td>
        <td className="num">
          {bytes(f.variants[0]?.size ?? 0)}
          <span className="cell-sub disc-variants">
            {f.variants.length} variant{f.variants.length === 1 ? "" : "s"}
          </span>
        </td>
        <td className="num disc-seeds-cell">{f.seeders}</td>
        <td>
          <ul className="disc-trackers" aria-label="Indexers with this release">
            {trackers.map((t) => {
              const listings = f.results.filter((r) => r.indexerId === t);
              return (
                <li key={t} className="code-tag">
                  {listings[0]?.indexer}
                  <TrackerFlags results={listings} short />
                </li>
              );
            })}
          </ul>
        </td>
        <td>
          <span className={`pill ${pill.tone}`} title={f.targetStatus.message}>
            {pill.text}
          </span>
        </td>
        <td className="actions">
          <button
            type="button"
            className="icon-button"
            aria-expanded={expanded}
            aria-controls={expanded ? detailsId : undefined}
            aria-label={`${expanded ? "Collapse" : "Expand"} ${p.resolution} ${kind} ${p.displayGroup || p.group || ""}`.replace(
              /\s+/g,
              " ",
            )}
            onClick={toggle}
          >
            {expanded ? (
              <ChevronDownIcon size={14} />
            ) : (
              <ChevronRightIcon size={14} />
            )}
          </button>
        </td>
      </tr>
      {expanded && (
        <tr className="disc-expand">
          <td colSpan={7} id={detailsId}>
            <FamilyExpansion
              jobId={jobId}
              family={f}
              selected={selected}
              targetName={targetName}
              onDetails={onDetails}
            />
          </td>
        </tr>
      )}
    </>
  );
}

function FamilyExpansion({
  jobId,
  family: f,
  selected,
  targetName,
  onDetails,
}: {
  jobId: string;
  family: Family;
  selected: number;
  targetName?: string;
  onDetails: () => void;
}) {
  const policy = f.policy ?? f.variants.find((v) => v.policy)?.policy;
  const byId = new Map(f.results.map((r) => [r.id, r]));
  const grouped = f.variants.map((v) => ({
    variant: v,
    results: v.resultIds
      .map((id) => byId.get(id))
      .filter((r): r is Result => !!r)
      .sort((a, b) => b.seeders - a.seeders),
  }));
  const listed = new Set(grouped.flatMap((g) => g.results.map((r) => r.id)));
  const rest = f.results.filter((r) => !listed.has(r.id));
  const row = (r: Result) => {
    const url = safeURL(r.infoUrl);
    return (
      <li key={r.id} className="disc-result">
        <span className="disc-result-idx">{r.indexer}</span>
        <span className="disc-result-title">
          {url ? (
            <a href={url} target="_blank" rel="noreferrer" title={r.title}>
              {r.title}
            </a>
          ) : (
            <span title={r.title}>{r.title}</span>
          )}
          <TrackerFlags results={[r]} short />
        </span>
        <span className="num">{bytes(r.size)}</span>
        <span className="num faint" title="Seeders">
          {r.seeders}
        </span>
        <span>
          <AddTorrent
            key={`${jobId}:${r.id}`}
            jobId={jobId}
            result={r}
            targetIndexerId={f.targetStatus.indexerId}
          />
        </span>
      </li>
    );
  };
  return (
    <div className="disc-expansion">
      <section className="disc-why" aria-label="Why">
        <h3>
          {policy
            ? targetName
              ? `Against ${targetName}`
              : "Target analysis"
            : "Target status"}
        </h3>
        {policy ? (
          <PolicySummary evaluation={policy} why />
        ) : (
          <p className="small">{f.targetStatus.message}</p>
        )}
        {policy && f.targetStatus.message && (
          <p className="hint">{f.targetStatus.message}</p>
        )}
      </section>
      <section className="disc-listings" aria-label="Listings">
        <ul>
          {grouped.map(({ variant, results }, i) => (
            <Fragment key={`${variant.size}:${i}`}>
              {grouped.length > 1 && (
                <li className="disc-variant">
                  <span className="num">{bytes(variant.size)}</span>
                  <span>
                    {variant.coverage}/{selected} indexers ·{" "}
                    {variant.confidence.toLowerCase()}
                  </span>
                  {variant.targetStatus?.candidate ? (
                    <span className="pill accent">Candidate</span>
                  ) : variant.targetStatus?.exactMatchFound ? (
                    <span className="pill ok">On target</span>
                  ) : null}
                </li>
              )}
              {results.map(row)}
            </Fragment>
          ))}
          {rest.map(row)}
        </ul>
        <div className="disc-listings-foot">
          <span className="hint">
            Exact bytes don’t verify contents. Check before cross-seeding.
          </span>
          <Button className="sm" onClick={onDetails}>
            Compare &amp; raw evidence
          </Button>
        </div>
      </section>
    </div>
  );
}
