import { RavenError } from "./raven-reactions";
import { Select } from "./ui/select";
import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, apiFailure } from "../api";
import type { Indexer, IndexerPreset } from "../types";
import { Button } from "./ui/button";

const storageKey = "ravenseed.indexerPreset";
function remember(id: string) {
  try {
    localStorage.setItem(storageKey, id);
  } catch {}
}
function remembered() {
  try {
    return localStorage.getItem(storageKey) ?? "";
  } catch {
    return "";
  }
}

export function availablePreset(preset: IndexerPreset, indexers: Indexer[]) {
  const ids = preset.indexerIds.filter((id) =>
    indexers.some((i) => i.id === id && i.enabled),
  );
  return {
    ids,
    target: ids.includes(preset.targetIndexerId) ? preset.targetIndexerId : 0,
    missing: preset.indexerIds.length - ids.length,
  };
}

export function IndexerPresets({
  indexers,
  selected,
  target,
  ready,
  apply,
  onActiveChange,
}: {
  indexers: Indexer[];
  selected: number[];
  target: number;
  ready: boolean;
  apply: (ids: number[], target: number) => void;
  /** Called with the active preset's name ("" for a custom selection). */
  onActiveChange?: (name: string) => void;
}) {
  const qc = useQueryClient();
  const presets = useQuery({
    queryKey: ["presets"],
    queryFn: () => api<IndexerPreset[]>("/presets"),
  });
  const [id, setID] = useState("");
  const [name, setName] = useState("");
  const [notice, setNotice] = useState("");
  const restored = useRef(false);
  const active = presets.data?.find((p) => p.id === id);
  const activeName = active?.name ?? "";
  useEffect(() => {
    onActiveChange?.(activeName);
  }, [activeName, onActiveChange]);
  const modified =
    active &&
    (active.targetIndexerId !== target ||
      [...active.indexerIds].sort().join() !== [...selected].sort().join());
  function choose(preset: IndexerPreset) {
    const selection = availablePreset(preset, indexers);
    apply(selection.ids, selection.target);
    setID(preset.id);
    setName(preset.name);
    remember(preset.id);
    setNotice(
      selection.missing
        ? `${selection.missing} unavailable indexer(s) omitted. Update the preset to save this selection.`
        : "",
    );
  }
  useEffect(() => {
    if (!ready || !presets.data || restored.current) return;
    restored.current = true;
    const preset = presets.data.find((p) => p.id === remembered());
    if (preset) choose(preset);
    // Runs once (restored guards it) as soon as presets and indexers are
    // ready; choose() reads this render's values.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ready, presets.data, indexers]);
  const save = useMutation({
    mutationFn: (preset: IndexerPreset) =>
      api<IndexerPreset>("/presets", {
        method: "PUT",
        body: JSON.stringify(preset),
      }),
    onSuccess: (preset) => {
      setID(preset.id);
      setName(preset.name);
      remember(preset.id);
      setNotice("Preset saved.");
      void qc.invalidateQueries({ queryKey: ["presets"] });
    },
  });
  const remove = useMutation({
    mutationFn: (presetID: string) =>
      api(`/presets/${presetID}`, { method: "DELETE" }).catch((error) => {
        // Already deleted (another tab or client): the outcome is the same.
        if (apiFailure(error).status !== 404) throw error;
      }),
    onSuccess: () => {
      setID("");
      setName("");
      remember("");
      setNotice("Preset deleted. Your current selection is unchanged.");
      void qc.invalidateQueries({ queryKey: ["presets"] });
    },
  });
  const pending = save.isPending || remove.isPending;
  return (
    <div className="preset-panel">
      <div className="preset-controls">
        <label className="preset-pick">
          Indexer preset
          <Select
            aria-label="Indexer preset"
            value={id}
            disabled={pending}
            onChange={(e) => {
              const preset = presets.data?.find((p) => p.id === e.target.value);
              if (preset) choose(preset);
              else {
                setID("");
                setName("");
                remember("");
                setNotice("");
              }
            }}
          >
            <option value="">Custom selection</option>
            {presets.data?.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
                {p.id === id && modified ? " (modified)" : ""}
              </option>
            ))}
          </Select>
        </label>
        <label className="preset-name">
          Preset name
          <input
            value={name}
            maxLength={80}
            placeholder="e.g. Movie trackers"
            onChange={(e) => setName(e.target.value)}
          />
        </label>
        <Button
          type="button"
          className="sm"
          disabled={pending || !name.trim() || !selected.length}
          onClick={() => {
            remove.reset();
            save.mutate({
              id: "",
              name: name.trim(),
              indexerIds: selected,
              targetIndexerId: target,
            });
          }}
        >
          Save new
        </Button>
        {active && (
          <>
            <Button
              type="button"
              className="sm"
              disabled={pending || !name.trim() || !selected.length}
              onClick={() => {
                remove.reset();
                save.mutate({
                  id,
                  name: name.trim(),
                  indexerIds: selected,
                  targetIndexerId: target,
                });
              }}
            >
              Update
            </Button>
            <Button
              type="button"
              className="sm danger"
              disabled={pending}
              onClick={() => {
                save.reset();
                remove.mutate(id);
              }}
            >
              Delete
            </Button>
          </>
        )}
      </div>
      <p className="hint">
        A preset saves the selected indexers and target. The last one used is
        restored in this browser.
      </p>
      {notice && (
        <p className="hint" role="status">
          {notice}
        </p>
      )}
      {(save.error || remove.error || presets.error) && (
        <RavenError>
          {(save.error || remove.error || presets.error)?.message}
        </RavenError>
      )}
    </div>
  );
}
