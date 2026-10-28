import { RavenError } from "./raven-reactions";
import { useMemo, useState } from "react";
import { useSettingsDraft } from "../hooks/use-settings-draft";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import type { EpisodeMapping, MatchingRules, Settings } from "../types";
import { CloseIcon } from "./icons";
import { ChipInput, SaveBar, SavedNotice } from "./settings-workspace";

const defaults: MatchingRules = {
  matchAlternateTitles: true,
  sizeTolerancePercent: 0.1,
  groupAliases: {},
  ignoredTags: [],
  episodeMappings: [],
};

function parseAliases(text: string): Record<string, string> {
  const aliases: Record<string, string> = Object.create(null);
  for (const line of text
    .split("\n")
    .map((s) => s.trim())
    .filter(Boolean)) {
    const parts = line.split("=").map((s) => s.trim().toUpperCase());
    if (parts.length !== 2 || !parts[0] || !parts[1])
      throw new Error("Use one group alias per line: OLDGROUP = NEWGROUP");
    if (Object.hasOwn(aliases, parts[0]))
      throw new Error(`Duplicate group alias: ${parts[0]}`);
    aliases[parts[0]] = parts[1];
  }
  return aliases;
}

function parseMappings(text: string): EpisodeMapping[] {
  return text
    .split("\n")
    .map((s) => s.trim())
    .filter(Boolean)
    .map((line) => {
      const match =
        /^(imdb:tt\d{7,10}|tmdb:[1-9]\d*)\s+(\d+)\s*=\s*S(\d+)E(\d+)$/i.exec(
          line,
        );
      if (!match)
        throw new Error(
          "Use one episode mapping per line: tmdb:1399 13 = S02E01",
        );
      const [kind, id] = match[1].toLowerCase().split(":");
      return {
        ...(kind === "imdb" ? { imdbId: id } : { tmdbId: Number(id) }),
        absolute: Number(match[2]),
        season: Number(match[3]),
        episode: Number(match[4]),
      };
    });
}

type AliasRow = { from: string; to: string };
type MappingRow = {
  id: string;
  absolute: string;
  season: string;
  episode: string;
};

function draftRules(saved: MatchingRules) {
  return {
    rules: {
      matchAlternateTitles: saved.matchAlternateTitles,
      sizeTolerancePercent: saved.sizeTolerancePercent,
    },
    aliases: Object.entries(saved.groupAliases).map(([from, to]): AliasRow => ({
      from,
      to,
    })),
    ignored: saved.ignoredTags,
    mappings: saved.episodeMappings.map((m): MappingRow => ({
      id: m.imdbId ? `imdb:${m.imdbId}` : `tmdb:${m.tmdbId}`,
      absolute: String(m.absolute),
      season: String(m.season),
      episode: String(m.episode),
    })),
  };
}

const blankAlias = (row: AliasRow) => !row.from.trim() && !row.to.trim();
const blankMapping = (row: MappingRow) =>
  !row.id.trim() && !row.absolute && !row.season && !row.episode;

/** Turns the editable rows into the PUT /settings/matching payload. */
export function rulesFromDraft(
  draft: ReturnType<typeof draftRules>,
): MatchingRules {
  return {
    ...draft.rules,
    groupAliases: parseAliases(
      draft.aliases
        .filter((r) => !blankAlias(r))
        .map((r) => `${r.from} = ${r.to}`)
        .join("\n"),
    ),
    ignoredTags: draft.ignored.map((s) => s.trim()).filter(Boolean),
    episodeMappings: parseMappings(
      draft.mappings
        .filter((r) => !blankMapping(r))
        .map(
          (r) =>
            `${r.id.trim()} ${r.absolute} = S${r.season || "?"}E${r.episode || "?"}`,
        )
        .join("\n"),
    ),
  };
}

export function MatchingSettings({ settings }: { settings?: Settings }) {
  const qc = useQueryClient();
  const saved = useMemo(
    () => (settings ? draftRules(settings.matching ?? defaults) : undefined),
    [settings],
  );
  const [draft, setDraft, accept, state] = useSettingsDraft(
    saved,
    draftRules(defaults),
  );
  const { rules, aliases, ignored, mappings } = draft;
  const [inputError, setInputError] = useState("");
  const save = useMutation({
    mutationFn: (value: MatchingRules) =>
      api<Settings>("/settings/matching", {
        method: "PUT",
        body: JSON.stringify(value),
      }),
    onSuccess: (data) => {
      accept(draftRules(data.matching ?? defaults));
      qc.setQueryData(["settings"], data);
      void qc.invalidateQueries({ queryKey: ["history"] });
    },
  });
  const setAlias = (i: number, patch: Partial<AliasRow>) =>
    setDraft((v) => ({
      ...v,
      aliases: v.aliases.map((r, n) => (n === i ? { ...r, ...patch } : r)),
    }));
  const setMapping = (i: number, patch: Partial<MappingRow>) =>
    setDraft((v) => ({
      ...v,
      mappings: v.mappings.map((r, n) => (n === i ? { ...r, ...patch } : r)),
    }));
  const error = inputError || save.error?.message;
  return (
    <section className="set-section" aria-labelledby="set-matching-title">
      <header className="set-head">
        <h2 id="set-matching-title">Matching rules</h2>
        <span className="hint">
          Apply to new searches. Saved searches keep the rules used at the time.
        </span>
      </header>
      <form
        className="set-form"
        onSubmit={(e) => {
          e.preventDefault();
          if (!settings) return;
          setInputError("");
          save.reset();
          try {
            save.mutate(rulesFromDraft(draft));
          } catch (error) {
            setInputError(
              error instanceof Error ? error.message : "Invalid rules",
            );
          }
        }}
      >
        {/* Saving replaces every rule list, so nothing is editable until the
            saved rules have loaded, nor while a save is in flight. */}
        <fieldset
          className="set-fieldset set-form"
          disabled={!settings || save.isPending}
        >
          <div className="panel pad stack">
            <label className="check-row">
              <input
                type="checkbox"
                checked={rules.matchAlternateTitles}
                onChange={(e) =>
                  setDraft((v) => ({
                    ...v,
                    rules: {
                      ...v.rules,
                      matchAlternateTitles: e.target.checked,
                    },
                  }))
                }
              />
              Match alternate titles with the same verified IMDb or TMDb
              identity
            </label>
            <div className="set-inline-field">
              <label htmlFor="set-size-tolerance">Size tolerance (%)</label>
              <input
                id="set-size-tolerance"
                type="number"
                min={0}
                max={1}
                step={0.01}
                required
                value={rules.sizeTolerancePercent}
                onChange={(e) =>
                  setDraft((v) => ({
                    ...v,
                    rules: {
                      ...v.rules,
                      sizeTolerancePercent: +e.target.value,
                    },
                  }))
                }
              />
              <span className="hint">
                0 disables it. Close sizes can be Strong; only identical byte
                sizes are Exact.
              </span>
            </div>
          </div>

          <div className="panel">
            <div className="panel-head">
              <h3>Group aliases</h3>
              <span className="hint">
                Treat an old group name as another. Chains resolve; cycles are
                rejected.
              </span>
            </div>
            {aliases.length > 0 && (
              <div className="table-wrap">
                <table className="data-table compact set-edit-table">
                  <thead>
                    <tr>
                      <th>Group</th>
                      <th className="set-col-arrow" aria-hidden="true" />
                      <th>Same as</th>
                      <th className="set-col-remove">
                        <span className="visually-hidden">Remove</span>
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {aliases.map((row, i) => {
                      const name = row.from.trim() || `row ${i + 1}`;
                      return (
                        <tr key={i}>
                          <td>
                            <input
                              className="mono"
                              aria-label={`Alias group ${i + 1}`}
                              placeholder="OLDGROUP"
                              value={row.from}
                              spellCheck={false}
                              onChange={(e) =>
                                setAlias(i, { from: e.target.value })
                              }
                            />
                          </td>
                          <td className="set-col-arrow" aria-hidden="true">
                            →
                          </td>
                          <td>
                            <input
                              className="mono"
                              aria-label={`Canonical group for ${name}`}
                              placeholder="CANONICALGROUP"
                              value={row.to}
                              spellCheck={false}
                              onChange={(e) =>
                                setAlias(i, { to: e.target.value })
                              }
                            />
                          </td>
                          <td className="set-col-remove">
                            <button
                              type="button"
                              className="icon-button"
                              aria-label={`Remove alias ${name}`}
                              onClick={() =>
                                setDraft((v) => ({
                                  ...v,
                                  aliases: v.aliases.filter((_, n) => n !== i),
                                }))
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
                setDraft((v) => ({
                  ...v,
                  aliases: [...v.aliases, { from: "", to: "" }],
                }))
              }
            >
              + Add alias
            </button>
          </div>

          <div className="panel">
            <div className="panel-head">
              <h3>Ignored tags</h3>
              <span className="hint">
                Cosmetic tags to ignore when matching. Quality, source, HDR,
                correction and episode tags can't be ignored.
              </span>
            </div>
            <div className="set-panel-body">
              <ChipInput
                label="Add ignored tag"
                placeholder="Add tag…"
                values={ignored}
                onChange={(next) => setDraft((v) => ({ ...v, ignored: next }))}
              />
            </div>
          </div>

          <div className="panel">
            <div className="panel-head">
              <h3>Absolute episode mappings</h3>
              <span className="hint">
                For anime. Use the same ID as the search; unmapped numbers stay
                separate.
              </span>
            </div>
            {mappings.length > 0 && (
              <div className="table-wrap">
                <table className="data-table compact set-edit-table">
                  <thead>
                    <tr>
                      <th>Media ID</th>
                      <th className="set-col-num">Absolute</th>
                      <th className="set-col-num">Season</th>
                      <th className="set-col-num">Episode</th>
                      <th className="set-col-remove">
                        <span className="visually-hidden">Remove</span>
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {mappings.map((row, i) => {
                      const name = `${row.id.trim() || "mapping"} ${row.absolute || i + 1}`;
                      return (
                        <tr key={i}>
                          <td>
                            <input
                              className="mono"
                              aria-label={`Media ID for mapping ${i + 1}`}
                              placeholder="tmdb:1399 or imdb:tt1234567"
                              value={row.id}
                              spellCheck={false}
                              onChange={(e) =>
                                setMapping(i, { id: e.target.value })
                              }
                            />
                          </td>
                          {(["absolute", "season", "episode"] as const).map(
                            (key) => (
                              <td key={key} className="set-col-num">
                                <input
                                  type="number"
                                  min={key === "season" ? 0 : 1}
                                  aria-label={`${key[0].toUpperCase()}${key.slice(1)} number for ${name}`}
                                  value={row[key]}
                                  onChange={(e) =>
                                    setMapping(i, { [key]: e.target.value })
                                  }
                                />
                              </td>
                            ),
                          )}
                          <td className="set-col-remove">
                            <button
                              type="button"
                              className="icon-button"
                              aria-label={`Remove mapping ${name}`}
                              onClick={() =>
                                setDraft((v) => ({
                                  ...v,
                                  mappings: v.mappings.filter(
                                    (_, n) => n !== i,
                                  ),
                                }))
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
                setDraft((v) => ({
                  ...v,
                  mappings: [
                    ...v.mappings,
                    { id: "", absolute: "", season: "", episode: "" },
                  ],
                }))
              }
            >
              + Add mapping
            </button>
          </div>

          {error && <RavenError>{error}</RavenError>}
          {save.isSuccess && !error && !state.dirty && (
            <SavedNotice>Matching rules saved.</SavedNotice>
          )}
          <SaveBar
            changes={state.changes}
            saving={save.isPending}
            onDiscard={() => {
              setInputError("");
              state.reset();
            }}
          />
        </fieldset>
      </form>
    </section>
  );
}

export { parseAliases, parseMappings };
