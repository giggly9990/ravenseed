import { RavenError } from "./raven-reactions";
import { useEffect, useState } from "react";
import { useSettingsDraft } from "../hooks/use-settings-draft";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import type { QuiInstance, TorrentSettings as Config } from "../types";
import { Select } from "./ui/select";
import { Button } from "./ui/button";
import { SaveBar, SavedNotice } from "./settings-workspace";

const empty: Config = { defaultInstanceId: 0, categories: [] };

/** Draft, queries and save mutation for PUT /settings/torrents. */
export function useTorrentDefaults() {
  const qc = useQueryClient();
  const saved = useQuery({
    queryKey: ["torrent-settings"],
    queryFn: () => api<Config>("/settings/torrents"),
  });
  const instances = useQuery({
    queryKey: ["qui-instances"],
    queryFn: () => api<QuiInstance[]>("/qui/instances"),
  });
  const [form, setForm, accept, draft] = useSettingsDraft<Config>(
    saved.data,
    empty,
  );
  const save = useMutation({
    mutationFn: () =>
      api<Config>("/settings/torrents", {
        method: "PUT",
        body: JSON.stringify(form),
      }),
    onSuccess: (v) => {
      accept(v);
      qc.setQueryData(["torrent-settings"], v);
    },
  });
  return {
    saved,
    instances,
    form,
    setForm,
    save,
    changes: draft.changes,
    reset: draft.reset,
  };
}

export type TorrentDefaultsState = ReturnType<typeof useTorrentDefaults>;

export function TorrentDefaultsFields({
  state,
}: {
  state: TorrentDefaultsState;
}) {
  const { form, setForm, saved, instances, save } = state;
  const [editing, setEditing] = useState(0);
  useEffect(() => {
    setEditing(form.defaultInstanceId);
  }, [form.defaultInstanceId]);
  const categories = useQuery({
    queryKey: ["qui-categories", editing],
    queryFn: () => api<string[]>(`/qui/instances/${editing}/categories`),
    enabled: editing > 0,
  });
  const rule = form.categories.find((c) => c.instanceId === editing);
  const names = [
    ...new Set([...(categories.data ?? []), ...(rule?.enabled ?? [])]),
  ].sort();
  const changeCategories = (enabled?: string[]) =>
    setForm((f) => ({
      ...f,
      categories: [
        ...f.categories.filter((c) => c.instanceId !== editing),
        ...(enabled ? [{ instanceId: editing, enabled }] : []),
      ],
    }));
  const choices = instances.data ?? [];
  const error = saved.error || instances.error || save.error;
  return (
    <div className="panel">
      <div className="panel-head">
        <h3>Torrents</h3>
        <span className="hint">
          Where new torrents go and which categories you can pick.
        </span>
      </div>
      <div className="set-panel-body stack">
        {/* Saving replaces every category rule, so nothing is editable until
            the saved defaults have loaded, nor while a save is in flight. */}
        <fieldset
          className="set-fieldset stack"
          disabled={!saved.isSuccess || save.isPending}
        >
          <div className="form-grid two">
            <label>
              Default qui instance
              <Select
                aria-label="Default qui instance"
                value={form.defaultInstanceId || ""}
                onChange={(e) =>
                  setForm({ ...form, defaultInstanceId: +e.target.value })
                }
              >
                <option value="">Choose each time</option>
                {form.defaultInstanceId > 0 &&
                  !choices.some((i) => i.id === form.defaultInstanceId) && (
                    <option value={form.defaultInstanceId}>
                      Instance {form.defaultInstanceId} (unavailable)
                    </option>
                  )}
                {choices.map((i) => (
                  <option key={i.id} value={i.id} disabled={!i.isActive}>
                    {i.name}
                    {!i.connected ? " (offline)" : ""}
                  </option>
                ))}
              </Select>
            </label>
            <label>
              Categories for instance
              <Select
                aria-label="Categories for instance"
                value={editing || ""}
                onChange={(e) => setEditing(+e.target.value)}
              >
                <option value="">Choose an instance</option>
                {[
                  ...new Set([
                    ...choices.map((i) => i.id),
                    ...form.categories.map((c) => c.instanceId),
                    ...(editing ? [editing] : []),
                  ]),
                ].map((id) => (
                  <option key={id} value={id}>
                    {choices.find((i) => i.id === id)?.name ??
                      `Instance ${id} (unavailable)`}
                  </option>
                ))}
              </Select>
            </label>
          </div>
          {editing > 0 && (
            <div className="set-categories">
              <label className="check-row">
                <input
                  type="checkbox"
                  checked={!!rule}
                  disabled={!rule && !categories.isSuccess}
                  onChange={(e) =>
                    changeCategories(
                      e.target.checked
                        ? [...(categories.data ?? [])]
                        : undefined,
                    )
                  }
                />
                Choose enabled categories
              </label>
              {rule ? (
                <label>
                  Enabled categories
                  <Select
                    aria-label="Enabled categories"
                    multiple
                    value={rule.enabled}
                    triggerLabel={
                      rule.enabled.length ? undefined : "No named categories"
                    }
                    onChange={(e) =>
                      changeCategories(
                        e.target.selectedOptions.map((o) => o.value),
                      )
                    }
                  >
                    {names.map((name) => (
                      <option key={name} value={name}>
                        {name}
                        {!categories.data?.includes(name) ? " (saved)" : ""}
                      </option>
                    ))}
                  </Select>
                </label>
              ) : (
                <p className="hint">
                  All categories are available, including ones added in qui
                  later. No category is always available.
                </p>
              )}
              {categories.isFetching && (
                <p role="status" className="hint">
                  Loading categories…
                </p>
              )}
              {categories.isError && (
                <RavenError
                  action={
                    <Button
                      type="button"
                      className="sm"
                      onClick={() => void categories.refetch()}
                    >
                      Reload categories
                    </Button>
                  }
                >
                  Could not load categories. Saved selections are retained.
                </RavenError>
              )}
            </div>
          )}
        </fieldset>
        {error && <RavenError>{error.message}</RavenError>}
      </div>
    </div>
  );
}

/** Standalone torrent defaults form (the Settings page embeds the fields in Defaults). */
export function TorrentSettings() {
  const state = useTorrentDefaults();
  return (
    <form
      className="set-form"
      onSubmit={(e) => {
        e.preventDefault();
        state.save.mutate();
      }}
    >
      <TorrentDefaultsFields state={state} />
      {state.save.isSuccess && state.changes === 0 && (
        <SavedNotice>Torrent defaults saved.</SavedNotice>
      )}
      <SaveBar
        label="Torrent defaults"
        changes={state.changes}
        saving={state.save.isPending}
        onDiscard={state.reset}
      />
    </form>
  );
}
