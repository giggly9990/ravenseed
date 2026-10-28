import { RavenError } from "./raven-reactions";
import { URLField } from "./url-privacy";
import { Select } from "./ui/select";
import { SecretInput } from "./ui/secret-input";
import { UploadPresets } from "./upload-presets";
import { useState } from "react";
import { useNow } from "../hooks/use-now";
import { shortRelativeTime } from "../lib/time";
import { useSettingsDraft } from "../hooks/use-settings-draft";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import type { Indexer, QuiInstance, UploadSettings as Config } from "../types";
import { Button } from "./ui/button";
import { CloseIcon, SpinnerIcon } from "./icons";
import { SaveBar, SavedNotice } from "./settings-workspace";

const empty: Config = {
  enabled: false,
  url: "",
  hasToken: false,
  trackers: [],
  paths: [],
};

/** Saved responses never carry the token; compare drafts on the same shape. */
const normalise = (value: Config): Config => ({
  ...value,
  token: value.token ?? "",
  clearToken: value.clearToken ?? false,
  flagPresets: value.flagPresets ?? [],
  defaultFlagPresetId: value.defaultFlagPresetId ?? "",
});

export function UploadSettings() {
  const qc = useQueryClient();
  const saved = useQuery({
    queryKey: ["upload-settings"],
    queryFn: () => api<Config>("/settings/uploads"),
    select: normalise,
  });
  const indexers = useQuery({
    queryKey: ["indexers"],
    queryFn: () => api<Indexer[]>("/indexers"),
  });
  const instances = useQuery({
    queryKey: ["qui-instances"],
    queryFn: () => api<QuiInstance[]>("/qui/instances"),
  });
  const [form, setForm, accept, draft] = useSettingsDraft(
    saved.data,
    normalise(empty),
  );
  const [message, setMessage] = useState("");
  const [tested, setTested] = useState<{ ok: boolean; at: number }>();
  const now = useNow(30_000, !!tested);
  const action = useMutation({
    mutationFn: ({ save, value }: { save: boolean; value: Config }) =>
      api<Config>(save ? "/settings/uploads" : "/settings/uploads/test", {
        method: save ? "PUT" : "POST",
        body: JSON.stringify(value),
      }),
    onSuccess: (value, variables) => {
      if (variables.save) {
        setMessage("Upload settings saved.");
        accept(normalise({ ...value, token: "", clearToken: false }));
        qc.setQueryData(["upload-settings"], value);
      } else {
        setMessage("");
        setTested({ ok: true, at: Date.now() });
      }
    },
    onError: (_, variables) => {
      if (!variables.save) setTested({ ok: false, at: Date.now() });
    },
  });
  const submit = (save: boolean) => {
    setMessage("");
    action.mutate({ save, value: form });
  };
  const indexerName = (id: number, row: number) =>
    indexers.data?.find((v) => v.id === id)?.name ??
    (id > 0 ? `indexer ${id}` : `row ${row + 1}`);
  const instanceName = (id: number, row: number) =>
    instances.data?.find((v) => v.id === id)?.name ??
    (id > 0 ? `instance ${id}` : `row ${row + 1}`);
  const error = saved.error || action.error;
  return (
    <section className="set-section" aria-labelledby="set-ua-title">
      <form
        className="set-form"
        onSubmit={(e) => {
          e.preventDefault();
          if (saved.isSuccess) submit(true);
        }}
      >
        {/* Saving replaces every list below, so nothing is editable until
            the saved settings have loaded, nor while a save is in flight. */}
        <fieldset
          className="set-fieldset set-form"
          disabled={
            !saved.isSuccess || (action.isPending && !!action.variables?.save)
          }
        >
          <header className="set-head">
            <h2 id="set-ua-title">Upload-Assistant</h2>
            {tested && (
              <span
                className={`pill ${tested.ok ? "ok" : "danger"}`}
                role="status"
              >
                <span
                  className={`status-dot ${tested.ok ? "online" : "offline"}`}
                  aria-hidden="true"
                />
                {tested.ok ? "Reachable" : "Unreachable"} · tested{" "}
                {shortRelativeTime(new Date(tested.at).toISOString(), now)}
              </span>
            )}
            <label className="switch-row set-head-end">
              Upload queue
              <button
                type="button"
                role="switch"
                className="switch"
                aria-checked={form.enabled}
                aria-label="Upload queue"
                onClick={() => setForm({ ...form, enabled: !form.enabled })}
              />
            </label>
          </header>
          <div className="panel pad set-connection">
            <div className="form-grid two">
              <URLField
                label="WebUI URL"
                placeholder="http://upload-assistant:5000"
                value={form.url}
                onChange={(e) => setForm({ ...form, url: e.target.value })}
              />
              <label>
                API token
                <SecretInput
                  placeholder={
                    form.clearToken
                      ? "Saved token will be removed"
                      : form.hasToken
                        ? "Saved · leave blank to keep"
                        : "Enter WebUI API token"
                  }
                  value={form.token || ""}
                  disabled={form.clearToken}
                  onChange={(e) => setForm({ ...form, token: e.target.value })}
                />
              </label>
            </div>
            <div className="row">
              <Button
                type="button"
                className="sm"
                disabled={action.isPending}
                onClick={() => submit(false)}
              >
                {action.isPending && !action.variables?.save && (
                  <SpinnerIcon size={14} className="spin" />
                )}
                Test connection
              </Button>
              {form.hasToken &&
                (form.clearToken ? (
                  <span className="hint">
                    Token will be removed and the queue paused on save.{" "}
                    <button
                      type="button"
                      className="link-button"
                      onClick={() =>
                        setForm({
                          ...form,
                          clearToken: false,
                          enabled: saved.data?.enabled ?? form.enabled,
                        })
                      }
                    >
                      Keep token
                    </button>
                  </span>
                ) : (
                  <button
                    type="button"
                    className="link-button small"
                    onClick={() =>
                      setForm({
                        ...form,
                        clearToken: true,
                        token: "",
                        enabled: false,
                      })
                    }
                  >
                    Remove saved token
                  </button>
                ))}
            </div>
          </div>

          <div className="panel">
            <div className="panel-head">
              <h3>Tracker codes</h3>
              <span className="hint">
                Which UA tracker each target indexer uploads to, e.g. AITHER.
              </span>
            </div>
            {form.trackers.length > 0 && (
              <div className="table-wrap">
                <table className="data-table compact set-edit-table">
                  <thead>
                    <tr>
                      <th>Indexer</th>
                      <th className="set-col-code">UA code</th>
                      <th className="set-col-remove">
                        <span className="visually-hidden">Remove</span>
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {form.trackers.map((t, i) => (
                      <tr key={i}>
                        <td>
                          <Select
                            aria-label={`Indexer for tracker row ${i + 1}`}
                            required
                            value={t.indexerId || ""}
                            onChange={(e) =>
                              setForm({
                                ...form,
                                trackers: form.trackers.map((v, n) =>
                                  n === i
                                    ? { ...v, indexerId: +e.target.value }
                                    : v,
                                ),
                              })
                            }
                          >
                            <option value="">Select indexer</option>
                            {t.indexerId > 0 &&
                              !indexers.data?.some(
                                (v) => v.id === t.indexerId,
                              ) && (
                                <option value={t.indexerId}>
                                  Indexer {t.indexerId}
                                </option>
                              )}
                            {indexers.data?.map((v) => (
                              <option key={v.id} value={v.id}>
                                {v.name}
                              </option>
                            ))}
                          </Select>
                        </td>
                        <td className="set-col-code">
                          <input
                            className="mono"
                            required
                            aria-label={`Tracker code for ${indexerName(t.indexerId, i)}`}
                            value={t.code}
                            placeholder="AITHER"
                            maxLength={40}
                            spellCheck={false}
                            onChange={(e) =>
                              setForm({
                                ...form,
                                trackers: form.trackers.map((v, n) =>
                                  n === i
                                    ? {
                                        ...v,
                                        code: e.target.value.toUpperCase(),
                                      }
                                    : v,
                                ),
                              })
                            }
                          />
                        </td>
                        <td className="set-col-remove">
                          <button
                            type="button"
                            className="icon-button"
                            aria-label={`Remove tracker ${indexerName(t.indexerId, i)}`}
                            onClick={() =>
                              setForm({
                                ...form,
                                trackers: form.trackers.filter(
                                  (_, n) => n !== i,
                                ),
                              })
                            }
                          >
                            <CloseIcon size={14} />
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
            <button
              type="button"
              className="link-button set-add"
              onClick={() =>
                setForm({
                  ...form,
                  trackers: [...form.trackers, { indexerId: 0, code: "" }],
                })
              }
            >
              + Add tracker
            </button>
          </div>

          <div className="panel">
            <div className="panel-head">
              <h3>Path mappings</h3>
              <span className="hint">
                Same files, seen from qBittorrent and from UA. Nothing is
                copied.
              </span>
            </div>
            {form.paths.length > 0 && (
              <div className="table-wrap">
                <table className="data-table compact set-edit-table">
                  <thead>
                    <tr>
                      <th className="set-col-instance">qui instance</th>
                      <th>qBittorrent path</th>
                      <th className="set-col-arrow" aria-hidden="true" />
                      <th>UA path</th>
                      <th className="set-col-remove">
                        <span className="visually-hidden">Remove</span>
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {form.paths.map((p, i) => {
                      const where = instanceName(p.instanceId, i);
                      return (
                        <tr key={i}>
                          <td className="set-col-instance">
                            <Select
                              aria-label={`qui instance for path row ${i + 1}`}
                              required
                              value={p.instanceId || ""}
                              onChange={(e) =>
                                setForm({
                                  ...form,
                                  paths: form.paths.map((v, n) =>
                                    n === i
                                      ? { ...v, instanceId: +e.target.value }
                                      : v,
                                  ),
                                })
                              }
                            >
                              <option value="">Select instance</option>
                              {p.instanceId > 0 &&
                                !instances.data?.some(
                                  (v) => v.id === p.instanceId,
                                ) && (
                                  <option value={p.instanceId}>
                                    Instance {p.instanceId}
                                  </option>
                                )}
                              {instances.data?.map((v) => (
                                <option key={v.id} value={v.id}>
                                  {v.name}
                                </option>
                              ))}
                            </Select>
                          </td>
                          <td>
                            <input
                              className="mono"
                              required
                              aria-label={`qBittorrent path for ${where}`}
                              placeholder="/downloads"
                              value={p.from}
                              spellCheck={false}
                              onChange={(e) =>
                                setForm({
                                  ...form,
                                  paths: form.paths.map((v, n) =>
                                    n === i
                                      ? { ...v, from: e.target.value }
                                      : v,
                                  ),
                                })
                              }
                            />
                          </td>
                          <td className="set-col-arrow" aria-hidden="true">
                            →
                          </td>
                          <td>
                            <input
                              className="mono"
                              required
                              aria-label={`Upload-Assistant path for ${where}`}
                              placeholder="/data/downloads"
                              value={p.to}
                              spellCheck={false}
                              onChange={(e) =>
                                setForm({
                                  ...form,
                                  paths: form.paths.map((v, n) =>
                                    n === i ? { ...v, to: e.target.value } : v,
                                  ),
                                })
                              }
                            />
                          </td>
                          <td className="set-col-remove">
                            <button
                              type="button"
                              className="icon-button"
                              aria-label={`Remove path for ${where}`}
                              onClick={() =>
                                setForm({
                                  ...form,
                                  paths: form.paths.filter((_, n) => n !== i),
                                })
                              }
                            >
                              <CloseIcon size={14} />
                            </button>
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            )}
            <button
              type="button"
              className="link-button set-add"
              onClick={() =>
                setForm({
                  ...form,
                  paths: [...form.paths, { instanceId: 0, from: "", to: "" }],
                })
              }
            >
              + Add path
            </button>
          </div>

          <UploadPresets value={form} onChange={setForm} />
          {error && <RavenError>{error.message}</RavenError>}
          {message && !draft.dirty && <SavedNotice>{message}</SavedNotice>}
          <SaveBar
            changes={draft.changes}
            saving={action.isPending && !!action.variables?.save}
            onDiscard={draft.reset}
            note="Pausing the queue never stops an upload already running in UA."
          />
        </fieldset>
      </form>
    </section>
  );
}
