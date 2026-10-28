import { useEffect, useRef } from "react";
import type { Monitor, MonitorCandidate } from "../../types";
import { bytes } from "../../api";
import { Button } from "../../components/ui/button";
import { CloseIcon } from "../../components/icons";
import { PolicySummary } from "../../components/policy-summary";
import { Link } from "@tanstack/react-router";
import {
  canSendToUA,
  candidatePill,
  identityLabels,
  mediaIDs,
  parsedTags,
} from "./format";
import { shortRelativeTime } from "../../lib/time";

export function CandidateDetail({
  candidate: c,
  monitor,
  instanceName,
  close,
  send,
  check,
  checking,
}: {
  candidate: MonitorCandidate | undefined;
  monitor: Monitor;
  instanceName?: string;
  close: () => void;
  send: () => void;
  check: () => void;
  checking: boolean;
}) {
  const panel = useRef<HTMLElement>(null);
  const id = c?.id;
  useEffect(() => {
    // On narrow screens the panel sits under the table; bring it into view.
    if (!id || typeof window.matchMedia !== "function") return;
    if (window.matchMedia("(max-width: 820px)").matches)
      panel.current?.scrollIntoView({ behavior: "smooth", block: "start" });
  }, [id]);
  if (!c)
    return (
      <aside className="panel mon-detail empty" aria-label="Candidate detail">
        <p className="muted">Select a release to see why it was flagged.</p>
      </aside>
    );
  const pill = candidatePill(c);
  const tags = parsedTags(c);
  const sendable = canSendToUA(c);
  return (
    <aside
      ref={panel}
      className="panel mon-detail"
      aria-label="Candidate detail"
    >
      <div className="mon-detail-head">
        <span className={`pill ${pill.tone}`}>{pill.label}</span>
        <span className="small faint">
          {c.lastCheckedAt
            ? `checked ${shortRelativeTime(c.lastCheckedAt)}`
            : "awaiting first check"}
        </span>
        <button
          type="button"
          className="icon-button mon-detail-close"
          aria-label="Close detail"
          onClick={close}
        >
          <CloseIcon size={15} />
        </button>
      </div>
      <h2 className="mon-detail-name mono">{c.name}</h2>
      {tags.length > 0 && (
        <div className="mon-tags">
          {tags.map((t) => (
            <span key={t} className="code-tag">
              {t}
            </span>
          ))}
        </div>
      )}
      <dl className="mon-dl">
        <dt>Size</dt>
        <dd className="num">
          {bytes(c.size)} · {c.size.toLocaleString()} B
        </dd>
        <dt>qui instance</dt>
        <dd>{instanceName ?? `Instance ${c.instanceId}`}</dd>
        {(c.identity || c.identityError) && (
          <>
            <dt>Media IDs</dt>
            <dd>
              {c.identity && mediaIDs(c.media) ? (
                <>
                  <span className="mono">{mediaIDs(c.media)}</span> · from{" "}
                  {c.identity.instance} {identityLabels[c.identity.source]}
                  {c.identity.importedFile ? (
                    <>
                      {" "}
                      · imported file{" "}
                      <span className="mono">{c.identity.importedFile}</span>
                    </>
                  ) : (
                    c.identity.viaHash && (
                      <>
                        {" "}
                        of cross-seed{" "}
                        <span className="mono">
                          {c.identity.viaHash.slice(0, 8)}
                        </span>
                      </>
                    )
                  )}
                  {c.identity.source === "arr_history" && (
                    <span className="pill ok mon-id-pill">verified</span>
                  )}
                </>
              ) : c.identityError ? (
                <span className="muted">
                  Sonarr/Radarr lookup failed; searched by title
                </span>
              ) : (
                <span className="muted">
                  Not found in Sonarr or Radarr; searched by title
                </span>
              )}
              {c.identityError && (
                <span className="small faint mon-id-error">
                  {c.identityError}
                </span>
              )}
            </dd>
          </>
        )}
        {c.targetQuery && (
          <>
            <dt>{monitor.targetIndexerName} search</dt>
            <dd>
              <span className="mono">{c.targetQuery}</span> ·{" "}
              {c.targetResultCount ?? 0} results, {c.scopedResultCount ?? 0} in
              scope
            </dd>
          </>
        )}
        <dt>Next check</dt>
        <dd>{shortRelativeTime(c.nextCheckAt) || "–"}</dd>
      </dl>
      {c.policy ? (
        <div className="mon-policy">
          <PolicySummary evaluation={c.policy} />
        </div>
      ) : (
        c.message && <p className="mon-policy small">{c.message}</p>
      )}
      <div className="mon-detail-actions">
        {sendable ? (
          <Button className="primary mon-grow" onClick={send}>
            Send to Upload-Assistant
          </Button>
        ) : c.uploadJobId ? (
          <Link to="/uploads" className="button soft mon-grow">
            View upload
          </Link>
        ) : null}
        <Button
          onClick={check}
          disabled={checking || c.status === "checking" || !monitor.enabled}
        >
          Check again
        </Button>
      </div>
      <p className="hint">
        RavenSeed verifies every file before handing the path to UA. UA runs its
        own dupe check.
      </p>
    </aside>
  );
}
