import { RavenError } from "./raven-reactions";
import { useEffect, useMemo, useState } from "react";
import { useSettingsDraft } from "../hooks/use-settings-draft";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { SecretInput } from "./ui/secret-input";
import { SaveBar, SavedNotice } from "./settings-workspace";
import { api } from "../api";
import type { MediaSnapshot, MetadataSettings, Settings } from "../types";
import { Button } from "./ui/button";

export function parseMediaInput(text: string) {
  const id = text.trim().toLowerCase();
  if (/^(imdb:)?tt\d{7,10}$/.test(id))
    return { imdbId: id.replace(/^imdb:/, "") };
  if (/^(tmdb:)?[1-9]\d*$/.test(id)) {
    const value = Number(id.replace(/^tmdb:/, ""));
    if (Number.isSafeInteger(value) && value <= 2147483647)
      return { tmdbId: value };
  }
  return null;
}

export function useMediaPreview(id: string, type: string, season: string) {
  const [settled, setSettled] = useState("");
  useEffect(() => {
    const timer = setTimeout(() => setSettled(id.trim()), 500);
    return () => clearTimeout(timer);
  }, [id]);
  return useQuery({
    queryKey: ["media-preview", settled, type, season],
    enabled: !!parseMediaInput(settled) && settled === id.trim(),
    queryFn: () =>
      api<MediaSnapshot>("/media/resolve", {
        method: "POST",
        body: JSON.stringify({
          id: settled,
          type,
          ...(type === "tv" && season !== "" ? { season: Number(season) } : {}),
        }),
      }),
  });
}

export function TMDbSettings({ settings }: { settings?: Settings }) {
  const qc = useQueryClient();
  const savedLanguage = settings?.metadata?.language;
  const savedDraft = useMemo(
    () =>
      savedLanguage === undefined
        ? undefined
        : { token: "", language: savedLanguage || "en-US", clearToken: false },
    [savedLanguage],
  );
  const [form, setForm, accept, draft] = useSettingsDraft(savedDraft, {
    token: "",
    language: "en-US",
    clearToken: false,
  });
  const { token, language, clearToken } = form;
  const [success, setSuccess] = useState("");
  const [tested, setTested] = useState<boolean>();
  const managed = !!settings?.metadata?.environmentManaged;
  const action = useMutation({
    mutationFn: ({
      save,
      value,
    }: {
      save: boolean;
      value: Partial<MetadataSettings>;
    }) =>
      api<Settings>(save ? "/settings/metadata" : "/settings/metadata/test", {
        method: save ? "PUT" : "POST",
        body: JSON.stringify(value),
      }),
    onSuccess: (value, variables) => {
      if (variables.save) {
        setSuccess("TMDb settings saved.");
        accept({
          token: "",
          language: value.metadata?.language || "en-US",
          clearToken: false,
        });
        qc.setQueryData(["settings"], value);
        void qc.invalidateQueries({ queryKey: ["media-preview"] });
      } else setTested(true);
    },
    onError: (_, variables) => {
      if (!variables.save) setTested(false);
    },
  });
  const submit = (save: boolean) => {
    setSuccess("");
    if (!save) setTested(undefined);
    action.mutate({ save, value: { token, language, clearToken } });
  };
  return (
    <section className="set-section" aria-labelledby="set-tmdb-title">
      <header className="set-head">
        <h2 id="set-tmdb-title">TMDb metadata</h2>
        {tested !== undefined && (
          <span className={`pill ${tested ? "ok" : "danger"}`} role="status">
            {tested ? "TMDb connection works" : "Connection failed"}
          </span>
        )}
      </header>
      <form
        className="set-form"
        onSubmit={(e) => {
          e.preventDefault();
          submit(true);
        }}
      >
        <div className="panel pad set-connection">
          <div className="form-grid two">
            <label>
              TMDb API key or Read Access Token
              <SecretInput
                value={token}
                disabled={managed || clearToken}
                placeholder={
                  clearToken
                    ? "Saved credential will be removed"
                    : settings?.metadata?.hasToken
                      ? "Saved · leave blank to keep"
                      : "Enter your TMDb API key or token"
                }
                onChange={(e) => setForm({ ...form, token: e.target.value })}
              />
            </label>
            <label>
              Metadata language
              <input
                className="mono"
                value={language}
                placeholder="en-US"
                pattern="[a-z]{2}(-[A-Z]{2})?"
                onChange={(e) => setForm({ ...form, language: e.target.value })}
              />
            </label>
          </div>
          <div className="row">
            <Button
              type="button"
              className="sm"
              disabled={action.isPending || clearToken}
              onClick={() => submit(false)}
            >
              Test connection
            </Button>
            {managed ? (
              <span className="hint">
                Credential configured through the environment.
              </span>
            ) : (
              settings?.metadata?.hasToken && (
                <label className="check-row small">
                  <input
                    type="checkbox"
                    checked={clearToken}
                    onChange={(e) =>
                      setForm({
                        ...form,
                        clearToken: e.target.checked,
                        token: "",
                      })
                    }
                  />
                  Remove saved credential
                </label>
              )
            )}
          </div>
          <p className="hint">
            Adds titles, artwork and season details. Direct-ID searches work
            without TMDb. This product uses the TMDB API but is not endorsed or
            certified by TMDB.
          </p>
        </div>
        {action.error && <RavenError>{action.error.message}</RavenError>}
        {success && !draft.dirty && <SavedNotice>{success}</SavedNotice>}
        <SaveBar
          changes={draft.changes}
          saving={action.isPending && !!action.variables?.save}
          onDiscard={draft.reset}
        />
      </form>
    </section>
  );
}
