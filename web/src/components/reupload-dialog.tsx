import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import type { UploadJob, UploadSettings } from "../types";
import { Button } from "./ui/button";
import { Dialog } from "./ui/dialog";
import { Select } from "./ui/select";
import { RavenError } from "./raven-reactions";

export function ReuploadDialog({
  job,
  close,
}: {
  job: UploadJob;
  close: () => void;
}) {
  const qc = useQueryClient();
  const [args, setArgs] = useState(job.args ?? "");
  const [preset, setPreset] = useState("");
  const [tmdb, setTMDb] = useState(job.media?.tmdbId?.toString() ?? "");
  const settings = useQuery({
    queryKey: ["upload-settings"],
    queryFn: () => api<UploadSettings>("/settings/uploads"),
  });
  const retry = useMutation({
    mutationFn: () =>
      api<UploadJob>(`/uploads/${job.id}/retry`, {
        method: "POST",
        body: JSON.stringify({
          attempt: job.attempt ?? 0,
          args,
          ...(job.media?.type ? { tmdbId: tmdb ? Number(tmdb) : 0 } : {}),
        }),
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["upload-queue"] });
      close();
    },
  });
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !retry.isPending) close();
      }}
      title="Reupload to target"
      description={`Reuse the existing download and upload to ${job.tracker}.`}
    >
      <form
        className="dialog-body"
        onSubmit={(event) => {
          event.preventDefault();
          retry.mutate();
        }}
      >
        <p className="mono small muted break">{job.title}</p>
        <p className="notice warn">
          Check the previous run in Upload-Assistant first. It may have uploaded
          before reporting an error or losing its connection.
        </p>
        <div className={job.media?.type ? "form-grid two" : undefined}>
          <label>
            UA flag preset
            <Select
              aria-label="UA flag preset"
              value={preset}
              onChange={(event) => {
                const id = event.target.value;
                setPreset(id);
                if (id)
                  setArgs(
                    settings.data?.flagPresets?.find((value) => value.id === id)
                      ?.args ?? "",
                  );
              }}
            >
              <option value="">Custom flags</option>
              {settings.data?.flagPresets?.map((value) => (
                <option key={value.id} value={value.id}>
                  {value.name}
                </option>
              ))}
            </Select>
          </label>
          {job.media?.type && (
            <label>
              TMDb ID
              <input
                type="number"
                min={1}
                max={2147483647}
                step={1}
                value={tmdb}
                placeholder="Optional"
                aria-describedby={`reupload-tmdb-${job.id}`}
                onChange={(event) => setTMDb(event.target.value)}
              />
            </label>
          )}
        </div>
        <label>
          UA flags
          <input
            className="mono"
            value={args}
            maxLength={4096}
            placeholder="--anon --modq"
            onChange={(event) => {
              setArgs(event.target.value);
              setPreset("");
            }}
          />
        </label>
        {job.media?.type && (
          <p className="hint" id={`reupload-tmdb-${job.id}`}>
            {job.media.type === "tv" ? "TV series" : "Movie"} ID. Sent to UA
            automatically when filled.
          </p>
        )}
        {(retry.error || settings.error) && (
          <RavenError>{(retry.error || settings.error)?.message}</RavenError>
        )}
        <div className="dialog-actions">
          <Button type="button" disabled={retry.isPending} onClick={close}>
            Cancel
          </Button>
          <Button type="submit" className="primary" disabled={retry.isPending}>
            {retry.isPending ? "Queueing..." : "Reupload"}
          </Button>
        </div>
      </form>
    </Dialog>
  );
}
