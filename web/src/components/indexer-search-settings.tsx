import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { ErrorNotice } from "../app/shared";
import { useSettingsDraft } from "../hooks/use-settings-draft";
import type { Indexer, IndexerSearchOverride, Settings } from "../types";
import { Button } from "./ui/button";
import { Select } from "./ui/select";
import { SaveBar, SavedNotice } from "./settings-workspace";

const empty: Record<string, IndexerSearchOverride> = {};
const defaults: IndexerSearchOverride = { movie: "auto", tv: "auto" };
export function IndexerSearchSettings({ settings }: { settings?: Settings }) {
  const qc = useQueryClient();
  const [draft, setDraft, accept, state] = useSettingsDraft(
    settings ? (settings.searchOverrides ?? empty) : undefined,
    empty,
  );
  const indexers = useQuery({
    queryKey: ["indexers"],
    queryFn: () => api<Indexer[]>("/indexers"),
    enabled: !!settings?.hasApiKey,
  });
  const save = useMutation({
    mutationFn: (value: Record<string, IndexerSearchOverride>) =>
      api<Settings>("/settings/search-overrides", {
        method: "PUT",
        body: JSON.stringify(value),
      }),
    onSuccess: (data) => {
      accept(data.searchOverrides ?? empty);
      qc.setQueryData(["settings"], data);
      void qc.invalidateQueries({ queryKey: ["indexers"] });
    },
  });
  const rows = [...(indexers.data ?? [])];
  for (const id of Object.keys(draft)) {
    if (!rows.some((row) => String(row.id) === id))
      rows.push({
        id: Number(id),
        name: `Indexer ${id} (unavailable)`,
        enabled: false,
        backend: "",
        capabilities: [],
      });
  }
  return (
    <section className="set-section" aria-labelledby="set-search-title">
      <header className="set-head">
        <h2 id="set-search-title">Per-indexer search</h2>
      </header>
      <form
        className="set-form"
        onSubmit={(event) => {
          event.preventDefault();
          save.mutate(draft);
        }}
      >
        <div className="panel">
          <div className="panel-head">
            <h3>Search method</h3>
            <span className="hint">
              Auto uses what the tracker advertises. If the chosen ID is
              missing, RavenSeed searches by title.
            </span>
            <div className="actions">
              <Button
                type="button"
                className="sm quiet"
                onClick={() => setDraft({})}
                disabled={!Object.keys(draft).length || save.isPending}
              >
                Reset all to Auto
              </Button>
            </div>
          </div>
          <ErrorNotice error={indexers.error ?? save.error} />
          {!settings?.hasApiKey && (
            <p className="table-empty">Connect qui to load your indexers.</p>
          )}
          {indexers.isLoading && (
            <p className="table-empty">Loading indexers…</p>
          )}
          {rows.length > 0 && (
            <fieldset
              className="set-fieldset"
              disabled={save.isPending || !settings}
            >
              <div className="table-wrap">
                <table className="data-table compact set-edit-table">
                  <thead>
                    <tr>
                      <th>Indexer</th>
                      <th className="set-col-method">Movies</th>
                      <th className="set-col-method">TV</th>
                    </tr>
                  </thead>
                  <tbody>
                    {rows.map((row) => {
                      const value = draft[row.id] ?? defaults;
                      const custom =
                        value.movie !== "auto" || value.tv !== "auto";
                      return (
                        <tr key={row.id}>
                          <td>
                            <span className="set-indexer-name">{row.name}</span>
                            {custom && (
                              <span className="pill quiet">Override</span>
                            )}
                          </td>
                          {(["movie", "tv"] as const).map((kind) => (
                            <td key={kind} className="set-col-method">
                              <Select
                                aria-label={`${row.name} ${kind === "movie" ? "movies" : "TV"} search method`}
                                value={value[kind]}
                                onChange={(event) =>
                                  setDraft((all) => {
                                    const next = {
                                      ...(all[row.id] ?? defaults),
                                      [kind]: event.target.value,
                                    };
                                    const { [row.id]: _, ...rest } = all;
                                    // Auto for both is no override at all.
                                    return next.movie === "auto" &&
                                      next.tv === "auto"
                                      ? rest
                                      : { ...rest, [row.id]: next };
                                  })
                                }
                              >
                                <option value="auto">Auto</option>
                                <option value="imdb">IMDb</option>
                                <option value="tmdb">TMDb</option>
                                {kind === "tv" && (
                                  <option value="tvdb">TVDb</option>
                                )}
                                <option value="title">Title</option>
                              </Select>
                            </td>
                          ))}
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            </fieldset>
          )}
          <p className="hint set-panel-foot">
            Monitors usually search by title because torrent names carry no
            verified IDs. Changes apply to new searches and monitor checks.
          </p>
        </div>
        {save.isSuccess && !state.dirty && (
          <SavedNotice>Search overrides saved.</SavedNotice>
        )}
        <SaveBar
          changes={state.changes}
          saving={save.isPending}
          onDiscard={state.reset}
        />
      </form>
    </section>
  );
}
