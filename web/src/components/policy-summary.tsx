import type { PolicyEvaluation } from "../types";
import { bytes, safeURL } from "../api";

const relationLabels: Record<string, string> = {
  equivalent: "Equivalent release",
  same_slot: "Same slot",
  policy_competitor: "Competes by policy rule",
  different_slot: "Different slot",
  unresolved: "Slot needs review",
};

const classificationLabels: Record<string, string> = {
  equivalent: "Already on target",
  missing_slot: "Missing slot",
  coexistence: "Can coexist",
  trump: "Trump",
  possible_trump: "Possible trump",
  not_eligible: "Not eligible",
  unevaluated: "Not evaluated",
};

export function policyLabel(classification: string) {
  return (
    classificationLabels[classification] ?? classification.replaceAll("_", " ")
  );
}

const value = (s: string) => s.replaceAll("_", " ");

function TargetResults({
  evaluation,
  brief,
}: {
  evaluation: PolicyEvaluation;
  brief?: boolean;
}) {
  if (!evaluation.targetResults?.length) {
    return evaluation.competingResultIds.length > 0 ? (
      <p className="hint">
        This older finding only saved target result IDs:{" "}
        {evaluation.competingResultIds.join(", ")}. Run a fresh search or Check
        again to see their release names.
      </p>
    ) : null;
  }
  return (
    <div className="policy-target-results">
      <p className="policy-label">
        {brief ? "On target now" : "Target releases considered"}
      </p>
      <ul>
        {evaluation.targetResults.map((result) => {
          const url = safeURL(result.infoUrl ?? "");
          const title = result.title || "Untitled release";
          return (
            <li key={result.id}>
              {url ? (
                <a
                  className="mono"
                  href={url}
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  {title}
                </a>
              ) : (
                <strong className="mono">{title}</strong>
              )}{" "}
              <span>
                · {result.indexer || "Target indexer"} · {bytes(result.size)} ·{" "}
                {relationLabels[result.relation] ?? "Related release"}
              </span>
              {!brief && (
                <small>
                  {result.size.toLocaleString()} bytes · Result ID {result.id}
                </small>
              )}
            </li>
          );
        })}
      </ul>
    </div>
  );
}

/**
 * Explains a target-policy finding.
 *
 * - default: everything, including evidence and the policy version.
 * - `compact`: one line with the classification and reason.
 * - `why`: the reason, improvements and regressions, and what the target
 *   already has; for inline "why" boxes.
 */
export function PolicySummary({
  evaluation,
  compact = false,
  why = false,
}: {
  evaluation: PolicyEvaluation;
  compact?: boolean;
  why?: boolean;
}) {
  const name = evaluation.classification.replaceAll("_", " ");
  if (compact && !why)
    return (
      <div className="policy-summary compact">
        <p>
          <strong>Target policy: {name}</strong> · {evaluation.reason}
        </p>
        {evaluation.slotKey && (
          <p className="hint">Observed slot: {evaluation.slotKey}</p>
        )}
      </div>
    );
  const changes = (
    <>
      {(evaluation.improvements.length > 0 ||
        evaluation.regressions.length > 0) && (
        <ul className="policy-changes">
          {evaluation.improvements.map((item, i) => (
            <li key={`up-${item.type}-${i}`}>
              <span className="policy-up" role="img" aria-label="Improvement">
                ↑
              </span>
              <span>
                {value(item.type)}{" "}
                <span className="mono">
                  {value(item.from)} → {value(item.to)}
                </span>
              </span>
            </li>
          ))}
          {evaluation.regressions.map((item, i) => (
            <li key={`down-${item.type}-${i}`}>
              <span className="policy-down" role="img" aria-label="Regression">
                ↓
              </span>
              <span>
                Regression: {value(item.type)}{" "}
                <span className="mono">
                  {value(item.from)} → {value(item.to)}
                </span>
              </span>
            </li>
          ))}
        </ul>
      )}
      {evaluation.missingEvidence.length > 0 && (
        <ul className="policy-notes">
          {evaluation.missingEvidence.map((item, i) => (
            <li key={`${item.rule}-${i}`}>{item.detail}</li>
          ))}
        </ul>
      )}
      {evaluation.reviewReasons.length > 0 && (
        <ul className="policy-notes">
          {evaluation.reviewReasons.map((reason) => (
            <li key={reason}>{reason}</li>
          ))}
        </ul>
      )}
    </>
  );
  if (why)
    return (
      <div className="policy-summary">
        <p>
          <strong>{policyLabel(evaluation.classification)}</strong> ·{" "}
          {evaluation.reason}
        </p>
        {changes}
        {evaluation.slotKey && (
          <p className="policy-slot">
            <span className="policy-eq" aria-hidden="true">
              =
            </span>{" "}
            Slot{" "}
            <span className="mono">
              {Object.values(evaluation.slotComponents ?? {}).length
                ? Object.values(evaluation.slotComponents ?? {})
                    .map(value)
                    .join(" · ")
                : evaluation.slotKey}
            </span>
          </p>
        )}
        <TargetResults evaluation={evaluation} brief />
      </div>
    );
  return (
    <div className="policy-summary">
      <p>
        <strong>Target policy: {name}</strong> · {evaluation.reason}
      </p>
      {evaluation.slotKey && (
        <p className="hint">Observed slot: {evaluation.slotKey}</p>
      )}
      <TargetResults evaluation={evaluation} />
      {changes}
      {evaluation.automaticEvidence.length > 0 && (
        <details>
          <summary>Evidence</summary>
          <ul className="policy-notes">
            {evaluation.automaticEvidence.map((item, i) => (
              <li key={`${item.rule}-${i}`}>{item.detail}</li>
            ))}
          </ul>
        </details>
      )}
      {evaluation.policySnapshotId && (
        <small className="faint">
          Policy version {evaluation.policySnapshotId}
        </small>
      )}
    </div>
  );
}
