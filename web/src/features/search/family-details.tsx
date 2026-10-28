import { useEffect, useRef, useState, type ReactNode } from "react";
import { useMutation } from "@tanstack/react-query";
import {
  CheckIcon,
  ExternalLinkIcon,
  ShieldCheckIcon,
} from "../../components/icons";
import { api, bytes, safeURL } from "../../api";
import type { Family, Result } from "../../types";
import { Dialog } from "../../components/ui/dialog";
import { TrackerFlags } from "../../components/tracker-flags";
import { PolicySummary } from "../../components/policy-summary";
import { AddTorrent } from "../../components/add-torrent";
import { label, ErrorNotice } from "../../app/shared";

export function FamilyDetails({
  jobId,
  family: f,
  close,
  showFamily,
}: {
  jobId: string;
  family: Family | null;
  close: () => void;
  showFamily: (family: Family) => void;
}) {
  // Read at response time: a related family that arrives after the dialog
  // closed must not reopen it.
  const open = useRef(false);
  useEffect(() => {
    open.current = !!f;
  }, [f]);
  const related = useMutation({
    mutationFn: (id: string) =>
      api<Family>(`/searches/${jobId}/families/${id}`),
    onSuccess: (family) => {
      if (open.current) showFamily(family);
    },
  });
  const reference = f?.variants[0]?.size ?? 0;
  const columns: { header: string; cell: (r: Result) => ReactNode }[] = [
    { header: "Indexer", cell: (r) => r.indexer },
    { header: "Tracker flags", cell: (r) => <TrackerFlags results={[r]} /> },
    {
      header: "Reported size",
      cell: (r) => (
        <span>
          {bytes(r.size)}
          <span className="cell-sub">{r.size.toLocaleString()} bytes</span>
        </span>
      ),
    },
    {
      header: "Difference",
      cell: (r) =>
        r.size > 0 && reference > 0
          ? `${(r.size - reference).toLocaleString()} bytes`
          : "Unknown",
    },
    { header: "Seeders", cell: (r) => r.seeders },
    { header: "Confidence", cell: (r) => r.confidence },
    {
      header: "Send",
      cell: (r) => (
        <AddTorrent
          key={`${jobId}:${r.id}`}
          jobId={jobId}
          result={r}
          targetIndexerId={f?.targetStatus.indexerId}
        />
      ),
    },
    {
      header: "Release",
      cell: (r) =>
        safeURL(r.infoUrl) ? (
          <a href={safeURL(r.infoUrl)} target="_blank" rel="noreferrer">
            Open <ExternalLinkIcon size={12} />
          </a>
        ) : (
          <span className="muted">Unavailable</span>
        ),
    },
  ];
  return (
    <Dialog
      open={!!f}
      onOpenChange={(open) => {
        if (!open) close();
      }}
      title={f?.canonical.title ?? "Release comparison"}
      description="Reported size variants and the evidence behind this family."
      wide
    >
      {f && (
        <>
          <div className="chips disc-detail-badges">
            <span className="pill">{f.confidence}</span>
            <span className="pill quiet">
              {f.totalCoverage} indexers in family
            </span>
            <span className="pill quiet">
              {f.variants.length} size variants
            </span>
          </div>
          {f.policy && (
            <div className="disc-detail-box">
              <PolicySummary evaluation={f.policy} />
            </div>
          )}
          <h3>Exact-size variants</h3>
          <div className="table-wrap">
            <table className="data-table compact disc-variant-table">
              <thead>
                <tr>
                  <th className="num">Bytes</th>
                  <th>Coverage</th>
                  <th>Confidence</th>
                  <th>Target</th>
                </tr>
              </thead>
              <tbody>
                {f.variants.map((v, i) => (
                  <tr key={`${v.size}:${i}`}>
                    <td className="num">{v.size.toLocaleString()}</td>
                    <td>{v.coverage} indexers</td>
                    <td>{v.confidence}</td>
                    <td>
                      {v.targetStatus?.message ?? f.targetStatus.message}
                      {v.targetStatus?.candidate && (
                        <>
                          {" "}
                          <span className="pill accent">
                            Potential candidate
                          </span>
                        </>
                      )}
                      {v.policy && (
                        <>
                          <PolicySummary evaluation={v.policy} compact />
                          <details>
                            <summary>View target releases and evidence</summary>
                            <PolicySummary evaluation={v.policy} />
                          </details>
                        </>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <h3>Listings</h3>
          <div className="table-wrap">
            <table className="data-table compact disc-listing-table">
              <thead>
                <tr>
                  {columns.map((c) => (
                    <th key={c.header}>{c.header}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {f.results.map((r, i) => (
                  <tr key={`${r.id}:${i}`}>
                    {columns.map((c) => (
                      <td key={c.header}>{c.cell(r)}</td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <h3>Why these releases match</h3>
          {!!f.relatedFamilies?.length && (
            <details className="disc-related">
              <summary>
                Related families with different or missing metadata
              </summary>
              {f.relatedFamilies.map((relation) => (
                <p key={relation.familyId}>
                  <button
                    type="button"
                    className="link-button"
                    onClick={() => related.mutate(relation.familyId)}
                    disabled={related.isPending}
                  >
                    Compare related family
                  </button>{" "}
                  · {relation.confidence}:{" "}
                  {relation.evidence.map(label).join(", ")}
                </p>
              ))}
            </details>
          )}
          <ErrorNotice error={related.error} />
          <ul className="disc-evidence">
            {f.evidence.map((e) => (
              <li key={e}>
                <CheckIcon size={13} />
                {label(e)}
              </li>
            ))}
          </ul>
          <div className="notice warn disc-detail-note">
            <ShieldCheckIcon size={18} />
            <span>
              Exact reported bytes do not verify torrent contents or file
              layout. Confirm compatibility before cross-seeding.
            </span>
          </div>
          <h3>Original results</h3>
          {f.results.map((r) => (
            <RawResult key={r.id} result={r} />
          ))}
        </>
      )}
    </Dialog>
  );
}

/** One original result; its JSON is only built once the section is opened. */
function RawResult({ result: r }: { result: Result }) {
  const [open, setOpen] = useState(false);
  return (
    <details
      className="disc-raw"
      onToggle={(e) => setOpen(e.currentTarget.open)}
    >
      <summary>
        {r.indexer} · {r.title}
      </summary>
      <p className="hint">
        {[
          ...r.evidence,
          ...(r.identityEvidence ?? []),
          ...(r.normalizationEvidence ?? []),
        ]
          .map(label)
          .join(" · ")}
      </p>
      {open && (
        <>
          <p className="hint">
            Download links and tracker keys stay on the server.
          </p>
          <pre>
            {JSON.stringify(
              {
                normalized: r.parsed,
                parser: r.parserOutput,
                original: r.raw,
              },
              null,
              2,
            )}
          </pre>
        </>
      )}
    </details>
  );
}
