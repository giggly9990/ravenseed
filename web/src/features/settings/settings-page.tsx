import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { ConnectionIcon, SpinnerIcon } from "../../components/icons";
import { api } from "../../api";
import type { Indexer, Settings } from "../../types";
import { Button } from "../../components/ui/button";
import { Select } from "../../components/ui/select";
import { SecretInput } from "../../components/ui/secret-input";
import { IndexerSearchSettings } from "../../components/indexer-search-settings";
import { MatchingSettings } from "../../components/matching-settings";
import { IndexerPolicies } from "../../components/indexer-policies";
import { UploadSettings } from "../../components/upload-settings";
import { ArrSettings } from "../../components/arr-settings";
import {
  TorrentDefaultsFields,
  useTorrentDefaults,
} from "../../components/torrent-settings";
import {
  isSettingsSection,
  SaveBar,
  SavedNotice,
  SettingsWorkspace,
  type SettingsSection,
} from "../../components/settings-workspace";
import { URLField } from "../../components/url-privacy";
import { TMDbSettings } from "../../components/media-metadata";
import { useSettingsDraft } from "../../hooks/use-settings-draft";
import { ErrorNotice } from "../../app/shared";

export function SettingsPage() {
  const search: { section?: unknown } = useSearch({ strict: false });
  const navigate = useNavigate();
  const section: SettingsSection = isSettingsSection(search.section)
    ? search.section
    : "qui";
  const saved = useQuery({
    queryKey: ["settings"],
    queryFn: () => api<Settings>("/settings"),
  });
  return (
    <SettingsWorkspace
      section={section}
      onSectionChange={(next) =>
        void navigate({
          to: "/settings",
          search: { section: next },
          replace: true,
        })
      }
      panels={{
        qui: <QuiConnection settings={saved.data} error={saved.error} />,
        uploads: <UploadSettings />,
        arr: <ArrSettings />,
        metadata: <TMDbSettings settings={saved.data} />,
        defaults: <SearchDefaults settings={saved.data} />,
        indexerSearch: <IndexerSearchSettings settings={saved.data} />,
        matching: <MatchingSettings settings={saved.data} />,
        policies: <IndexerPolicies ready={!!saved.data?.hasApiKey} />,
      }}
    />
  );
}

/** The PUT /settings payload: the saved settings without matching rules. */
function connectionPayload(saved: Settings | undefined, patch: object) {
  const value: Partial<Settings> = { ...saved, ...patch };
  delete value.matching;
  return value;
}

function QuiConnection({
  settings,
  error,
}: {
  settings?: Settings;
  error: unknown;
}) {
  const client = useQueryClient();
  const quiUrl = settings?.quiUrl ?? "";
  const initial = useMemo(() => ({ quiUrl, apiKey: "" }), [quiUrl]);
  const [form, setForm, accept, draft] = useSettingsDraft(
    settings ? initial : undefined,
    initial,
  );
  const [tested, setTested] = useState<{ ok: boolean; message: string }>();
  const [success, setSuccess] = useState("");
  const managed = !!settings?.environmentManaged;
  const test = useMutation({
    mutationFn: () =>
      api<{ indexers: Indexer[] }>("/settings/test", {
        method: "POST",
        body: JSON.stringify(connectionPayload(settings, form)),
      }),
    onSuccess: (data) =>
      setTested({
        ok: true,
        message: `Connected · ${data.indexers.filter((i) => i.enabled).length} enabled indexers`,
      }),
    onError: () => setTested({ ok: false, message: "Unreachable" }),
  });
  const save = useMutation({
    mutationFn: () =>
      api<Settings>("/settings", {
        method: "PUT",
        body: JSON.stringify(connectionPayload(settings, form)),
      }),
    onSuccess: (data) => {
      setSuccess("Settings saved. Your connection is ready.");
      accept({ quiUrl: data.quiUrl, apiKey: "" });
      client.setQueryData(["settings"], data);
      void client.invalidateQueries();
    },
  });
  const edit = (patch: Partial<typeof form>) => {
    setSuccess("");
    setTested(undefined);
    setForm({ ...form, ...patch });
  };
  return (
    <section className="set-section" aria-labelledby="set-qui-title">
      <header className="set-head">
        <h2 id="set-qui-title">qui</h2>
        {tested && (
          <span className={`pill ${tested.ok ? "ok" : "danger"}`}>
            {tested.message}
          </span>
        )}
      </header>
      <form
        className="set-form"
        onSubmit={(e) => {
          e.preventDefault();
          // The PUT carries the other saved settings: never send it unloaded.
          if (settings) save.mutate();
        }}
      >
        <div className="panel pad set-connection">
          <div className="form-grid two">
            <URLField
              label="qui URL"
              placeholder="http://host.docker.internal:7476"
              required
              value={form.quiUrl}
              disabled={managed || !settings}
              onChange={(e) => edit({ quiUrl: e.target.value })}
            />
            <label>
              API key
              <SecretInput
                placeholder={
                  settings?.hasApiKey
                    ? "Saved · leave blank to keep"
                    : "Enter your qui API key"
                }
                value={form.apiKey}
                disabled={managed || !settings}
                onChange={(e) => edit({ apiKey: e.target.value })}
              />
            </label>
          </div>
          <div className="row">
            <Button
              type="button"
              className="sm"
              disabled={test.isPending}
              onClick={() => test.mutate()}
            >
              {test.isPending ? (
                <SpinnerIcon size={14} className="spin" />
              ) : (
                <ConnectionIcon size={14} />
              )}
              Test connection
            </Button>
            <span className="hint">
              {managed
                ? "Configured through environment variables."
                : "Saved keys are never returned by the API. Environment variables and Docker secrets also work."}
            </span>
          </div>
        </div>
        <ErrorNotice error={error ?? test.error ?? save.error} />
        {success && <SavedNotice>{success}</SavedNotice>}
        <SaveBar
          label="qui"
          changes={draft.changes}
          saving={save.isPending}
          onDiscard={() => {
            draft.reset();
            setTested(undefined);
          }}
        />
      </form>
    </section>
  );
}

function SearchDefaults({ settings }: { settings?: Settings }) {
  const client = useQueryClient();
  const target = settings?.defaultTargetIndexer ?? 0;
  const coverage = settings?.defaultMinCoverage ?? 2;
  const initial = useMemo(
    () => ({ defaultTargetIndexer: target, defaultMinCoverage: coverage }),
    [target, coverage],
  );
  const [form, setForm, accept, draft] = useSettingsDraft(
    settings ? initial : undefined,
    initial,
  );
  const torrents = useTorrentDefaults();
  const [success, setSuccess] = useState("");
  const indexers = useQuery({
    queryKey: ["indexers"],
    queryFn: () => api<Indexer[]>("/indexers"),
    enabled: !!settings?.hasApiKey,
  });
  const save = useMutation({
    mutationFn: () =>
      api<Settings>("/settings", {
        method: "PUT",
        body: JSON.stringify(
          connectionPayload(settings, { ...form, apiKey: "" }),
        ),
      }),
    onSuccess: (data) => {
      accept({
        defaultTargetIndexer: data.defaultTargetIndexer,
        defaultMinCoverage: data.defaultMinCoverage,
      });
      client.setQueryData(["settings"], data);
    },
  });
  const changes = draft.changes + torrents.changes;
  const saving = save.isPending || torrents.save.isPending;
  return (
    <section className="set-section" aria-labelledby="set-defaults-title">
      <header className="set-head">
        <h2 id="set-defaults-title">Defaults</h2>
        <span className="hint">
          Used when you start a search or add a torrent.
        </span>
      </header>
      <form
        className="set-form"
        onSubmit={async (e) => {
          e.preventDefault();
          setSuccess("");
          const jobs: Promise<unknown>[] = [];
          if (draft.dirty) jobs.push(save.mutateAsync());
          if (torrents.changes) jobs.push(torrents.save.mutateAsync());
          const results = await Promise.allSettled(jobs);
          if (results.every((r) => r.status === "fulfilled"))
            setSuccess("Defaults saved.");
        }}
      >
        <div className="panel">
          <div className="panel-head">
            <h3>Search</h3>
          </div>
          {/* The PUT carries the other saved settings: no edits before load. */}
          <fieldset
            className="set-fieldset"
            disabled={!settings || save.isPending}
          >
            <div className="form-grid two set-panel-body">
              <label>
                Default target indexer
                <Select
                  aria-label="Default target indexer"
                  value={form.defaultTargetIndexer}
                  onChange={(e) =>
                    setForm({ ...form, defaultTargetIndexer: +e.target.value })
                  }
                >
                  <option value={0}>No default target</option>
                  {form.defaultTargetIndexer > 0 &&
                    !indexers.data?.some(
                      (i) => i.id === form.defaultTargetIndexer,
                    ) && (
                      <option value={form.defaultTargetIndexer}>
                        Indexer {form.defaultTargetIndexer}
                      </option>
                    )}
                  {(indexers.data ?? [])
                    .filter((i) => i.enabled)
                    .map((i) => (
                      <option key={i.id} value={i.id}>
                        {i.name}
                      </option>
                    ))}
                </Select>
              </label>
              <label>
                Minimum coverage
                <input
                  type="number"
                  min={1}
                  max={1000}
                  value={form.defaultMinCoverage}
                  onChange={(e) =>
                    setForm({ ...form, defaultMinCoverage: +e.target.value })
                  }
                />
                <span className="hint">
                  Indexers a release must appear on to be listed.
                </span>
              </label>
            </div>
          </fieldset>
        </div>
        <TorrentDefaultsFields state={torrents} />
        <ErrorNotice error={indexers.error ?? save.error} />
        {success && changes === 0 && <SavedNotice>{success}</SavedNotice>}
        <SaveBar
          label="Defaults"
          changes={changes}
          saving={saving}
          onDiscard={() => {
            draft.reset();
            torrents.reset();
          }}
        />
      </form>
    </section>
  );
}
