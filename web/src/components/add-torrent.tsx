import { RavenError } from "./raven-reactions";
import { Select } from "./ui/select";
import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CheckIcon, DownloadIcon, SpinnerIcon } from "./icons";
import { api, bytes } from "../api";
import type {
  QuiInstance,
  Result,
  TorrentSettings,
  UploadSettings,
} from "../types";
import { Button } from "./ui/button";
import { Dialog } from "./ui/dialog";
import { TrackerFlags } from "./tracker-flags";

export function AddTorrent({
  jobId,
  result,
  targetIndexerId,
  addPath,
  allowUpload = true,
  available = result.hasDownloadUrl,
}: {
  jobId?: string;
  result: Result;
  targetIndexerId?: number;
  /** Endpoint to POST to; defaults to the saved search result's add route. */
  addPath?: string;
  /** Offer "upload to target after download". */
  allowUpload?: boolean;
  /** Whether the release can be added; defaults to having a download link. */
  available?: boolean;
}) {
  const qc = useQueryClient();
  // Without a saved search there is no default add route.
  const endpoint =
    addPath ?? (jobId ? `/searches/${jobId}/results/${result.id}/add` : "");
  const [open, setOpen] = useState(false);
  const [instanceId, setInstanceID] = useState(0);
  const [instanceChosen, setInstanceChosen] = useState(false);
  const [chosenPreset, setChosenPreset] = useState<string | null>(null);
  const preferences = useQuery({
    queryKey: ["torrent-settings"],
    queryFn: () => api<TorrentSettings>("/settings/torrents"),
    enabled: open,
    staleTime: 0,
  });
  const [category, setCategory] = useState("");
  const [savePath, setSavePath] = useState("");
  const [tags, setTags] = useState("");
  const [paused, setPaused] = useState(true);
  const [upload, setUpload] = useState(false);
  const uploadSettings = useQuery({
    queryKey: ["upload-settings"],
    queryFn: () => api<UploadSettings>("/settings/uploads"),
    enabled: open,
  });
  const tracker = uploadSettings.data?.trackers?.find(
    (t) => t.indexerId === targetIndexerId,
  );
  const presetID =
    chosenPreset ?? uploadSettings.data?.defaultFlagPresetId ?? "";
  const preset = uploadSettings.data?.flagPresets?.find(
    (p) => p.id === presetID,
  );
  const canUpload =
    !!uploadSettings.data?.enabled &&
    !!tracker &&
    (!presetID || !!preset) &&
    targetIndexerId !== result.indexerId &&
    !!uploadSettings.data?.paths?.some((p) => p.instanceId === instanceId);
  const instances = useQuery({
    queryKey: ["qui-instances"],
    queryFn: () => api<QuiInstance[]>("/qui/instances"),
    enabled: open,
    staleTime: 0,
  });
  useEffect(() => {
    if (
      !open ||
      instanceChosen ||
      !preferences.isSuccess ||
      !instances.isSuccess
    )
      return;
    const initial = instances.data.find(
      (i) =>
        i.id === preferences.data.defaultInstanceId &&
        i.connected &&
        i.isActive,
    );
    setInstanceID(initial?.id ?? 0);
    setInstanceChosen(true);
  }, [
    open,
    instanceChosen,
    preferences.isSuccess,
    preferences.data,
    instances.isSuccess,
    instances.data,
  ]);
  const categoryRule = preferences.data?.categories?.find(
    (c) => c.instanceId === instanceId,
  );
  const categoryAllowed =
    !category || !categoryRule || categoryRule.enabled.includes(category);
  const add = useMutation({
    mutationFn: (options: {
      instanceId: number;
      category: string;
      savePath: string;
      tags: string;
      paused: boolean;
      uploadAfterDownload?: boolean;
      uploadPresetId?: string;
    }) =>
      api<{ added: number; uploadJobId?: string }>(endpoint, {
        method: "POST",
        body: JSON.stringify(options),
      }),
    onSuccess: (data) => {
      if (data.uploadJobId)
        void qc.invalidateQueries({ queryKey: ["upload-queue"] });
    },
  });
  const destination = instances.data?.find((i) => i.id === instanceId);
  const categories = useQuery({
    queryKey: ["qui-categories", instanceId],
    queryFn: () => api<string[]>(`/qui/instances/${instanceId}/categories`),
    enabled: open && !!destination?.connected && destination.isActive,
    staleTime: 0,
  });
  const availableCategories =
    categories.isError && categoryRule ? categoryRule.enabled : categories.data;
  return (
    <>
      <Button
        className="sm"
        disabled={!available || !endpoint || add.isSuccess}
        onClick={() => setOpen(true)}
        title={!available ? "qui did not supply a download link" : undefined}
      >
        {add.isSuccess ? <CheckIcon size={13} /> : <DownloadIcon size={13} />}
        {add.isSuccess ? "Added" : "Add to qui"}
      </Button>
      <Dialog
        open={open}
        onOpenChange={(value) => {
          if (!add.isPending) setOpen(value);
        }}
        title="Add torrent to qui"
        description="Choose the qBittorrent instance and options for this release."
      >
        <div className="picked-release">
          <strong className="mono break">{result.title}</strong>
          <p>
            {result.indexer} · {bytes(result.size)}{" "}
            <TrackerFlags results={[result]} />
          </p>
        </div>
        {add.isSuccess ? (
          <div className="notice success" role="status">
            <CheckIcon size={18} />
            qui accepted the torrent. Check its status in qui.
            {add.data?.uploadJobId && <a href="/uploads">View upload queue</a>}
          </div>
        ) : (
          <form
            className="add-torrent-form"
            onSubmit={(e) => {
              e.preventDefault();
              if (
                !add.isPending &&
                preferences.isSuccess &&
                categoryAllowed &&
                destination?.connected &&
                destination.isActive &&
                (!upload || canUpload)
              )
                add.mutate({
                  instanceId,
                  category,
                  savePath,
                  tags,
                  paused,
                  ...(upload
                    ? { uploadAfterDownload: true, uploadPresetId: presetID }
                    : {}),
                });
            }}
          >
            <label>
              Destination instance
              <Select
                aria-label="Destination instance"
                required
                value={instanceId || ""}
                disabled={
                  instances.isFetching ||
                  preferences.isFetching ||
                  add.isPending
                }
                onChange={(e) => {
                  setInstanceID(+e.target.value);
                  setInstanceChosen(true);
                  setCategory("");
                  setUpload(false);
                }}
              >
                <option value="">Choose an instance</option>
                {instances.data?.map((i) => (
                  <option
                    key={i.id}
                    value={i.id}
                    disabled={!i.connected || !i.isActive}
                  >
                    {i.name}
                    {!i.isActive
                      ? " (disabled)"
                      : !i.connected
                        ? " (offline)"
                        : ""}
                  </option>
                ))}
              </Select>
            </label>
            {instances.isFetching && (
              <p className="hint" role="status">
                Loading qui instances...
              </p>
            )}
            {instances.data?.length === 0 && (
              <p className="hint">Add a qBittorrent instance in qui first.</p>
            )}
            <label>
              Category
              {categories.isError && !categoryRule ? (
                <input
                  value={category}
                  maxLength={256}
                  disabled={add.isPending}
                  placeholder="Enter a category, optional"
                  onChange={(e) => setCategory(e.target.value)}
                />
              ) : (
                <Select
                  aria-label="Category"
                  value={category}
                  disabled={
                    add.isPending ||
                    !destination?.connected ||
                    !destination.isActive ||
                    categories.isFetching
                  }
                  onChange={(e) => setCategory(e.target.value)}
                >
                  <option value="">No category</option>
                  {category &&
                    categoryAllowed &&
                    !availableCategories?.includes(category) && (
                      <option value={category}>{category}</option>
                    )}
                  {availableCategories
                    ?.filter(
                      (name) =>
                        !categoryRule || categoryRule.enabled.includes(name),
                    )
                    .map((name) => (
                      <option key={name} value={name}>
                        {name}
                      </option>
                    ))}
                </Select>
              )}
            </label>
            {!categoryAllowed && (
              <RavenError>
                The selected category is now disabled. Choose another category.
              </RavenError>
            )}
            {categories.isFetching && (
              <p className="hint" role="status">
                Loading categories...
              </p>
            )}
            {categories.isSuccess && categories.data.length === 0 && (
              <p className="hint">
                No categories configured for this instance.
              </p>
            )}
            {categories.isError && (
              <div>
                <RavenError>
                  {categoryRule
                    ? "Could not load categories. Showing enabled categories from Settings."
                    : "Could not load categories. Enter one manually or retry."}
                </RavenError>
                <Button
                  type="button"
                  disabled={add.isPending}
                  onClick={() => void categories.refetch()}
                >
                  Reload categories
                </Button>
              </div>
            )}
            <label>
              Save path
              <input
                value={savePath}
                maxLength={4096}
                disabled={add.isPending}
                placeholder="Use client default"
                onChange={(e) => setSavePath(e.target.value)}
              />
              <span className="hint">
                Path on the selected qBittorrent host.
              </span>
            </label>
            <label>
              Tags
              <input
                value={tags}
                maxLength={1024}
                disabled={add.isPending}
                placeholder="Comma-separated, optional"
                onChange={(e) => setTags(e.target.value)}
              />
            </label>
            {allowUpload && (
              <>
                <label className="check-row">
                  <input
                    type="checkbox"
                    checked={upload}
                    disabled={add.isPending || !canUpload}
                    onChange={(e) => {
                      setUpload(e.target.checked);
                      setPaused(!e.target.checked);
                    }}
                  />
                  Upload to target after download
                  {tracker ? ` (${tracker.code})` : ""}
                </label>
                {!canUpload && (
                  <p className="hint">
                    To queue an upload, select an instance with a path mapping
                    and a mapped search target different from the source
                    tracker. Configure Upload-Assistant in Settings.
                  </p>
                )}
              </>
            )}
            {upload && (
              <label>
                UA flag preset
                <Select
                  aria-label="UA flag preset"
                  value={presetID}
                  disabled={add.isPending || uploadSettings.isFetching}
                  onChange={(e) => setChosenPreset(e.target.value)}
                >
                  <option value="">No preset</option>
                  {presetID && !preset && (
                    <option value={presetID} disabled>
                      Preset unavailable
                    </option>
                  )}
                  {uploadSettings.data?.flagPresets?.map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.name}
                    </option>
                  ))}
                </Select>
                {preset?.args && (
                  <span className="hint">
                    <code>{preset.args}</code>
                  </span>
                )}
              </label>
            )}
            {upload && (
              <p className="hint">
                Uploads to {tracker?.code} after all files finish downloading.
                The selected preset is saved with this job.{" "}
                {paused
                  ? "Waiting until you resume the torrent in qui."
                  : "The torrent will start downloading immediately."}
              </p>
            )}
            <label className="check-row">
              <input
                type="checkbox"
                checked={paused}
                disabled={add.isPending}
                onChange={(e) => setPaused(e.target.checked)}
              />
              Add paused
            </label>
            <p className="hint">
              Adds this listing using its indexer download link. Existing files
              are checked by qBittorrent.
            </p>
            {(preferences.error || instances.error || add.error) && (
              <RavenError>
                {(preferences.error || instances.error || add.error)?.message}
              </RavenError>
            )}
            {preferences.isError && (
              <Button type="button" onClick={() => void preferences.refetch()}>
                Reload torrent defaults
              </Button>
            )}
            {instances.error && (
              <Button type="button" onClick={() => void instances.refetch()}>
                Reload instances
              </Button>
            )}
            <Button
              className="primary"
              type="submit"
              disabled={
                add.isPending ||
                !preferences.isSuccess ||
                !categoryAllowed ||
                (upload && !canUpload) ||
                !destination?.connected ||
                !destination.isActive
              }
            >
              {add.isPending ? (
                <SpinnerIcon className="spin" size={16} />
              ) : (
                <DownloadIcon size={16} />
              )}
              {add.isPending ? "Sending to qui" : "Add torrent"}
            </Button>
          </form>
        )}
      </Dialog>
    </>
  );
}
