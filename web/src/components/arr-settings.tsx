import { useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import type { ArrInstance, ArrSettings as Config } from "../types";
import { useSettingsDraft } from "../hooks/use-settings-draft";
import { RavenError } from "./raven-reactions";
import { URLField } from "./url-privacy";
import { Select } from "./ui/select";
import { SecretInput } from "./ui/secret-input";
import { Button } from "./ui/button";
import { CloseIcon, ConnectionIcon, SpinnerIcon } from "./icons";
import { SaveBar, SavedNotice } from "./settings-workspace";

/** Saved responses never carry keys; compare drafts on the same shape. */
const normalise = (value: Config): Config => ({
  instances: (value.instances ?? []).map((v) => ({ ...v, apiKey: "" })),
});

const blank = (kind: ArrInstance["kind"]): ArrInstance => ({
  id: "",
  name: kind === "sonarr" ? "Sonarr" : "Radarr",
  kind,
  url: "",
  apiKey: "",
  hasApiKey: false,
  enabled: true,
});

export function ArrSettings() {
  const qc = useQueryClient();
  const saved = useQuery({
    queryKey: ["arr-settings"],
    queryFn: () => api<Config>("/settings/arr"),
    select: normalise,
  });
  const [form, setForm, accept, draft] = useSettingsDraft(saved.data, {
    instances: [],
  } as Config);
  const [message, setMessage] = useState("");
  const [tested, setTested] = useState<Record<number, boolean>>({});
  /** Bumped on every row change so in-flight test results can't land on another row. */
  const revision = useRef(0);
  const save = useMutation({
    mutationFn: (value: Config) =>
      api<Config>("/settings/arr", {
        method: "PUT",
        body: JSON.stringify(value),
      }),
    onSuccess: (value) => {
      setMessage(
        "Sonarr and Radarr settings saved. Cached torrent IDs were cleared.",
      );
      setTested({});
      accept(normalise(value));
      qc.setQueryData(["arr-settings"], value);
    },
  });
  const test = useMutation({
    mutationFn: ({
      value,
    }: {
      row: number;
      revision: number;
      value: ArrInstance;
    }) =>
      api("/settings/arr/test", {
        method: "POST",
        body: JSON.stringify(value),
      }),
    onSettled: (_, error, { row, revision: at }) => {
      if (at === revision.current)
        setTested((all) => ({ ...all, [row]: !error }));
    },
  });
  const edit = (row: number, patch: Partial<ArrInstance>) => {
    revision.current++;
    setMessage("");
    setTested(({ [row]: _, ...rest }) => rest);
    setForm({
      instances: form.instances.map((v, n) =>
        n === row ? { ...v, ...patch } : v,
      ),
    });
  };
  const add = (kind: ArrInstance["kind"]) => {
    revision.current++;
    setMessage("");
    setForm({ instances: [...form.instances, blank(kind)] });
  };
  const remove = (row: number) => {
    revision.current++;
    setMessage("");
    setTested({});
    setForm({ instances: form.instances.filter((_, n) => n !== row) });
  };
  return (
    <section className="set-section" aria-labelledby="set-arr-title">
      <form
        className="set-form"
        onSubmit={(e) => {
          e.preventDefault();
          if (!saved.isSuccess) return;
          setMessage("");
          save.mutate(form);
        }}
      >
        <header className="set-head">
          <h2 id="set-arr-title">Sonarr &amp; Radarr</h2>
          <span className="hint">
            Resolves IMDb, TMDb and TVDb IDs for existing qui torrents so
            tracker monitors can search by ID.
          </span>
        </header>
        {/* Saving replaces the whole instance list, so nothing is editable
            until the saved list has loaded, nor while a save is in flight
            (its response replaces the draft). */}
        <fieldset
          className="set-fieldset set-form"
          disabled={!saved.isSuccess || save.isPending}
        >
          {form.instances.map((v, i) => (
            <div
              key={v.id || `new-${i}`}
              className="panel pad set-connection"
              role="group"
              aria-label={v.name || `Instance ${i + 1}`}
            >
              <div className="row">
                <label className="switch-row">
                  Enabled
                  <button
                    type="button"
                    role="switch"
                    className="switch"
                    aria-checked={v.enabled}
                    aria-label={`Enabled: ${v.name || `instance ${i + 1}`}`}
                    onClick={() => edit(i, { enabled: !v.enabled })}
                  />
                </label>
                {tested[i] !== undefined && (
                  <span
                    className={`pill ${tested[i] ? "ok" : "danger"}`}
                    role="status"
                  >
                    {tested[i] ? "Connected" : "Connection failed"}
                  </span>
                )}
                <button
                  type="button"
                  className="icon-button set-head-end"
                  aria-label={`Remove ${v.name || `instance ${i + 1}`}`}
                  onClick={() => remove(i)}
                >
                  <CloseIcon size={14} />
                </button>
              </div>
              <div className="form-grid two">
                <label>
                  Name
                  <input
                    required
                    maxLength={80}
                    value={v.name}
                    onChange={(e) => edit(i, { name: e.target.value })}
                  />
                </label>
                <label>
                  Type
                  <Select
                    aria-label={`Type of ${v.name || `instance ${i + 1}`}`}
                    value={v.kind}
                    onChange={(e) =>
                      edit(i, { kind: e.target.value as ArrInstance["kind"] })
                    }
                  >
                    <option value="sonarr">Sonarr (TV)</option>
                    <option value="radarr">Radarr (movies)</option>
                  </Select>
                </label>
                <URLField
                  label="URL"
                  placeholder={
                    v.kind === "sonarr"
                      ? "http://sonarr:8989"
                      : "http://radarr:7878"
                  }
                  required
                  value={v.url}
                  onChange={(e) => edit(i, { url: e.target.value })}
                />
                <label>
                  API key
                  <SecretInput
                    value={v.apiKey ?? ""}
                    placeholder={
                      v.hasApiKey
                        ? "Saved · leave blank to keep"
                        : "Settings > General > API Key"
                    }
                    onChange={(e) => edit(i, { apiKey: e.target.value })}
                  />
                </label>
              </div>
              <div className="row">
                <Button
                  type="button"
                  className="sm"
                  disabled={test.isPending}
                  onClick={() =>
                    test.mutate({
                      row: i,
                      revision: revision.current,
                      value: v,
                    })
                  }
                >
                  {test.isPending && test.variables?.row === i ? (
                    <SpinnerIcon size={14} className="spin" />
                  ) : (
                    <ConnectionIcon size={14} />
                  )}
                  Test connection
                </Button>
              </div>
            </div>
          ))}
          <div className="panel pad set-connection">
            <div className="row set-arr-add">
              <button
                type="button"
                className="link-button set-add"
                onClick={() => add("sonarr")}
              >
                + Add Sonarr
              </button>
              <button
                type="button"
                className="link-button set-add"
                onClick={() => add("radarr")}
              >
                + Add Radarr
              </button>
            </div>
            <p className="hint">
              Read-only: RavenSeed only reads download history, parses names and
              looks up titles. A torrent&apos;s download history is matched by
              its hash and counts as a verified identity. Name matches are used
              for searching but still need manual review before a trump. IDs are
              cached per torrent and reused by later checks.
            </p>
          </div>
          {(saved.error || save.error || test.error) && (
            <RavenError>
              {(saved.error ?? save.error ?? test.error)?.message}
            </RavenError>
          )}
          {message && !draft.dirty && <SavedNotice>{message}</SavedNotice>}
          <SaveBar
            changes={draft.changes}
            saving={save.isPending}
            onDiscard={() => {
              draft.reset();
              revision.current++;
              setTested({});
            }}
          />
        </fieldset>
      </form>
    </section>
  );
}
