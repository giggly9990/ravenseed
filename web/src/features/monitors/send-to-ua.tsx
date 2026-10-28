import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { api, bytes } from "../../api";
import type { MonitorCandidate, UploadJob, UploadSettings } from "../../types";
import { Button } from "../../components/ui/button";
import { Select } from "../../components/ui/select";
import { Dialog } from "../../components/ui/dialog";
import { ErrorNotice } from "../../app/shared";
import { CheckIcon, ErrorIcon } from "../../components/icons";
import {
  identityLabels,
  mediaIDs,
  nameMatchIdentity,
  trustedIdentity,
} from "./format";

export function SendToUA({
  candidate,
  targetIndexerId,
  targetName,
  instanceName,
  close,
}: {
  candidate: MonitorCandidate;
  targetIndexerId: number;
  /** Target tracker (indexer) name shown in the readiness checks. */
  targetName?: string;
  instanceName?: string;
  close: () => void;
}) {
  const qc = useQueryClient();
  const trusted = trustedIdentity(candidate);
  // A library name match is prefilled too, but the server sends its IDs only
  // when the user confirms them.
  const nameMatch = nameMatchIdentity(candidate);
  const prefill = trusted || nameMatch ? (candidate.media.tmdbId ?? 0) : 0;
  // Follows the candidate until edited, so a refetch that drops a trusted
  // identity also drops its prefilled ID.
  const [enteredTmdbId, setTmdbId] = useState<number>();
  const tmdbId = enteredTmdbId ?? prefill;
  // Only blanking a prefilled ID clears the resolved IDs; an identity with
  // no TMDb ID (IMDb only) keeps them when the field is left empty.
  const cleared = prefill > 0 && enteredTmdbId === 0;
  const [confirmed, setConfirmed] = useState(false);
  // Name-match IDs reach UA only when kept unchanged; a typed or cleared ID
  // replaces them, so no confirmation is needed then.
  const needsConfirm = nameMatch && !cleared && tmdbId === prefill;
  const target = targetName || "The target tracker";
  const [presetId, setPresetId] = useState<string | null>(null);
  const settings = useQuery({
    queryKey: ["upload-settings"],
    queryFn: () => api<UploadSettings>("/settings/uploads"),
  });
  const submit = useMutation({
    mutationFn: () =>
      api<UploadJob>(`/monitor-candidates/${candidate.id}/upload`, {
        method: "POST",
        body: JSON.stringify({
          instanceId: candidate.instanceId,
          tmdbId,
          // A cleared prefill must reach the server, which otherwise keeps
          // the resolved IDs; UA then identifies the release itself.
          ...(cleared ? { clearMediaIds: true } : {}),
          ...(needsConfirm && confirmed ? { confirmMediaIds: true } : {}),
          uploadPresetId: presetId ?? settings.data?.defaultFlagPresetId ?? "",
        }),
      }),
    onSuccess: () => {
      // The candidate moves to "queued" and a job joins the upload queue.
      for (const key of [
        "monitor-candidates",
        "candidate-count",
        "upload-queue",
      ])
        void qc.invalidateQueries({ queryKey: [key] });
    },
  });
  const ready = !!settings.data?.enabled;
  const mapped = !!settings.data?.trackers.some(
    (t) => t.indexerId === targetIndexerId,
  );
  const path = !!settings.data?.paths.some(
    (p) => p.instanceId === candidate.instanceId,
  );
  const checks = [
    {
      ok: ready,
      done: "Upload-Assistant is enabled",
      todo: "Enable Upload-Assistant in Settings first.",
    },
    {
      ok: mapped,
      done: `${target} is mapped to a UA tracker code`,
      todo: `Map ${target} to a UA tracker code in Settings.`,
    },
    {
      ok: path,
      done: "This qui instance has a path mapping",
      todo: "Add a path mapping for this qui instance in Settings.",
    },
  ];
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !submit.isPending) close();
      }}
      title="Send to Upload-Assistant"
      description="Review this torrent before Upload-Assistant starts."
    >
      <div className="dialog-body">
        <div className="mon-picked">
          <span className="cell-title break">{candidate.name}</span>
          <span className="cell-sub">
            {bytes(candidate.size)} ·{" "}
            {instanceName ?? `qui instance ${candidate.instanceId}`}
            {candidate.message ? ` · ${candidate.message}` : ""}
          </span>
        </div>
        {submit.isSuccess ? (
          <div className="notice success" role="status">
            Queued for file verification.{" "}
            <Link to="/uploads">View upload queue</Link>
          </div>
        ) : (
          <form
            onSubmit={(e) => {
              e.preventDefault();
              submit.mutate();
            }}
            className="dialog-body"
          >
            <div className="form-grid two">
              <label>
                TMDb ID (optional, recommended for UA)
                <input
                  type="number"
                  min={0}
                  value={tmdbId || ""}
                  onChange={(e) => setTmdbId(+e.target.value)}
                  placeholder="Leave blank for UA to identify"
                />
                {trusted && candidate.identity && (
                  <span className="hint">
                    From {candidate.identity.instance}{" "}
                    {identityLabels[candidate.identity.source]}
                  </span>
                )}
                {nameMatch && candidate.identity && (
                  <span className="hint">
                    From a {candidate.identity.instance || "Sonarr/Radarr"}{" "}
                    {identityLabels[candidate.identity.source]}, not download
                    history
                  </span>
                )}
              </label>
              <label>
                UA flags preset
                <Select
                  aria-label="UA flags preset"
                  value={presetId ?? settings.data?.defaultFlagPresetId ?? ""}
                  onChange={(e) => setPresetId(e.target.value)}
                >
                  <option value="">No preset</option>
                  {settings.data?.flagPresets?.map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.name}
                    </option>
                  ))}
                </Select>
              </label>
            </div>
            {needsConfirm && (
              <label className="check-row">
                <input
                  type="checkbox"
                  checked={confirmed}
                  disabled={submit.isPending}
                  onChange={(e) => setConfirmed(e.target.checked)}
                />
                Confirm these IDs ({mediaIDs(candidate.media)}) are this release
              </label>
            )}
            {settings.data && (
              <ul className="mon-checks" aria-label="Readiness">
                {checks.map((c) =>
                  c.ok ? (
                    <li key={c.done} className="ok">
                      <CheckIcon size={14} />
                      {c.done}
                    </li>
                  ) : (
                    <li key={c.todo} className="bad">
                      <ErrorIcon size={14} />
                      {c.todo}
                    </li>
                  ),
                )}
              </ul>
            )}
            <p className="hint">
              RavenSeed verifies the torrent and every file before handing its
              path to UA. UA runs its own dupe check.
            </p>
            <div className="dialog-actions">
              <Button
                type="button"
                className="quiet"
                onClick={close}
                disabled={submit.isPending}
              >
                Cancel
              </Button>
              <Button
                className="primary"
                type="submit"
                disabled={
                  submit.isPending ||
                  !ready ||
                  !mapped ||
                  !path ||
                  (needsConfirm && !confirmed)
                }
              >
                Queue upload
              </Button>
            </div>
          </form>
        )}
        <ErrorNotice error={settings.error || submit.error} />
      </div>
    </Dialog>
  );
}
