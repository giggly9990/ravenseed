import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useId,
  useRef,
  useState,
  type KeyboardEvent,
  type ReactNode,
} from "react";
import { URLPrivacyProvider, URLPrivacyToggle } from "./url-privacy";
import { CloseIcon, SpinnerIcon } from "./icons";
import { Select } from "./ui/select";
import { Button } from "./ui/button";

export const settingsGroups = [
  {
    title: "Connections",
    items: [
      { id: "qui", title: "qui" },
      { id: "uploads", title: "Upload-Assistant" },
      { id: "arr", title: "Sonarr & Radarr" },
      { id: "metadata", title: "TMDb metadata" },
    ],
  },
  {
    title: "Searching",
    items: [
      { id: "defaults", title: "Defaults" },
      { id: "indexerSearch", title: "Per-indexer search" },
      { id: "matching", title: "Matching rules" },
    ],
  },
  {
    title: "Uploading",
    items: [{ id: "policies", title: "Indexer policies" }],
  },
] as const;

export type SettingsSection =
  (typeof settingsGroups)[number]["items"][number]["id"];
const sections: { id: SettingsSection; title: string }[] =
  settingsGroups.flatMap((g) => [...g.items]);

export function isSettingsSection(value: unknown): value is SettingsSection {
  return sections.some((s) => s.id === value);
}

type SectionInfo = {
  id: SettingsSection | "";
  title: string;
  report: (id: SettingsSection, changes: number) => void;
};
const SectionContext = createContext<SectionInfo>({
  id: "",
  title: "",
  report: () => {},
});

/** Tell the settings navigation how many unsaved edits this section has (-1: not reporting). */
export function useReportDirty(changes: number) {
  const { id, report } = useContext(SectionContext);
  useEffect(() => {
    if (id && changes >= 0) report(id, changes);
  }, [id, report, changes]);
  const reporting = changes >= 0;
  useEffect(
    () => () => {
      if (id && reporting) report(id, 0);
    },
    [id, report, reporting],
  );
}

/**
 * Sticky "N unsaved changes" bar. Rendered inside the section's form so the
 * Save button submits it (and native validation runs). Hidden when clean.
 */
export function SaveBar({
  changes,
  onDiscard,
  saving = false,
  note,
  label,
  report = true,
}: {
  changes: number;
  onDiscard: () => void;
  saving?: boolean;
  note?: ReactNode;
  /** Defaults to the enclosing section's title. */
  label?: string;
  /** Set false when the section reports its own total via useReportDirty. */
  report?: boolean;
}) {
  const { title } = useContext(SectionContext);
  useReportDirty(report ? changes : -1);
  if (changes <= 0) return null;
  return (
    <div className="save-bar" role="region" aria-label="Unsaved changes">
      <span className="status-dot" aria-hidden="true" />
      <span>
        {changes} unsaved {changes === 1 ? "change" : "changes"}
        {(label ?? title) ? ` in ${label ?? title}` : ""}
      </span>
      {note && <span className="hint">{note}</span>}
      <div className="actions">
        <Button
          type="button"
          className="quiet"
          disabled={saving}
          onClick={onDiscard}
        >
          Discard
        </Button>
        <Button type="submit" className="primary" disabled={saving}>
          {saving && <SpinnerIcon size={14} className="spin" />}
          Save
        </Button>
      </div>
    </div>
  );
}

/** Short-lived success message (role=status) shown after a save. */
export function SavedNotice({ children }: { children: ReactNode }) {
  return (
    <p role="status" className="notice success set-saved">
      {children}
    </p>
  );
}

/** Free-form list of short values shown as removable chips. */
export function ChipInput({
  label,
  values,
  onChange,
  placeholder = "Add…",
  transform = (v) => v,
  mono = true,
  invalidHint = "Some entries were not added.",
}: {
  label: string;
  values: string[];
  onChange: (values: string[]) => void;
  placeholder?: string;
  /** Normalises an entry; return "" to reject it. */
  transform?: (value: string) => string;
  mono?: boolean;
  /** Shown while rejected entries stay in the box for correction. */
  invalidHint?: string;
}) {
  const [text, setText] = useState("");
  const [rejected, setRejected] = useState(false);
  const hintId = useId();
  const add = () => {
    const pieces = text
      .split(",")
      .map((v) => v.trim())
      .filter(Boolean);
    const next = pieces.map(transform).filter((v) => v && !values.includes(v));
    const bad = pieces.filter((v) => !transform(v));
    if (next.length) onChange([...values, ...new Set(next)]);
    // Keep what could not be added instead of silently dropping it.
    setText(bad.join(", "));
    setRejected(bad.length > 0);
  };
  return (
    <div className="set-chip-input">
      {values.map((value) => (
        <span key={value} className={`set-chip${mono ? " mono" : ""}`}>
          {value}
          <button
            type="button"
            aria-label={`Remove ${value}`}
            onClick={() => onChange(values.filter((v) => v !== value))}
          >
            <CloseIcon size={11} />
          </button>
        </span>
      ))}
      <input
        aria-label={label}
        aria-invalid={rejected || undefined}
        aria-describedby={rejected ? hintId : undefined}
        placeholder={placeholder}
        value={text}
        onChange={(e) => {
          setText(e.target.value);
          setRejected(false);
        }}
        onBlur={add}
        onKeyDown={(e) => {
          if (e.key === "Enter" || e.key === ",") {
            e.preventDefault();
            add();
          } else if (e.key === "Backspace" && !text && values.length)
            onChange(values.slice(0, -1));
        }}
      />
      {rejected && (
        <span id={hintId} className="hint" role="status">
          {invalidHint}
        </span>
      )}
    </div>
  );
}

export function SettingsWorkspace({
  panels,
  section,
  onSectionChange,
}: {
  panels: Record<SettingsSection, ReactNode>;
  /** Controlled section (e.g. from `?section=`). */
  section?: SettingsSection;
  onSectionChange?: (section: SettingsSection) => void;
}) {
  const [active, setOwn] = useState<SettingsSection>(section ?? "qui");
  useEffect(() => {
    if (section) setOwn(section);
  }, [section]);
  const setActive = (next: SettingsSection) => {
    setOwn(next);
    if (next !== section) onSectionChange?.(next);
  };
  const [dirty, setDirty] = useState<Partial<Record<SettingsSection, number>>>(
    {},
  );
  // Panels mount on first visit (each loads its own data) and then stay
  // mounted, so unsaved drafts survive switching sections.
  const [visited, setVisited] = useState<ReadonlySet<SettingsSection>>(
    () => new Set([active]),
  );
  useEffect(() => {
    setVisited((all) => (all.has(active) ? all : new Set(all).add(active)));
  }, [active]);
  const report = useCallback((id: SettingsSection, changes: number) => {
    setDirty((all) =>
      (all[id] ?? 0) === changes ? all : { ...all, [id]: changes },
    );
  }, []);
  const id = useId();
  const buttons = useRef<(HTMLButtonElement | null)[]>([]);
  const onKeyDown = (event: KeyboardEvent, index: number) => {
    let next = index;
    if (event.key === "ArrowDown") next = (index + 1) % sections.length;
    else if (event.key === "ArrowUp")
      next = (index + sections.length - 1) % sections.length;
    else if (event.key === "Home") next = 0;
    else if (event.key === "End") next = sections.length - 1;
    else return;
    event.preventDefault();
    setActive(sections[next].id);
    buttons.current[next]?.focus();
  };
  return (
    <URLPrivacyProvider>
      <div className="set-workspace">
        <aside className="set-nav">
          <h1>Settings</h1>
          <div
            className="set-tabs"
            role="tablist"
            aria-label="Settings sections"
            aria-orientation="vertical"
          >
            {settingsGroups.map((group, g) => (
              <div key={group.title} role="presentation" className="set-group">
                {/* A tablist may only hold tabs: the title is hidden here
                    and read as each tab's description instead. */}
                <span
                  className="set-group-title"
                  id={`${id}-group-${g}`}
                  aria-hidden="true"
                >
                  {group.title}
                </span>
                {group.items.map((item) => {
                  const index = sections.findIndex((s) => s.id === item.id);
                  const changed = (dirty[item.id] ?? 0) > 0;
                  return (
                    <button
                      key={item.id}
                      ref={(node) => {
                        buttons.current[index] = node;
                      }}
                      type="button"
                      role="tab"
                      className="set-tab"
                      id={`${id}-${item.id}-tab`}
                      aria-controls={`${id}-${item.id}-panel`}
                      aria-selected={active === item.id}
                      aria-describedby={`${id}-group-${g}`}
                      tabIndex={active === item.id ? 0 : -1}
                      onClick={() => setActive(item.id)}
                      onKeyDown={(event) => onKeyDown(event, index)}
                    >
                      <span className="set-tab-label">{item.title}</span>
                      {changed && (
                        <>
                          <span
                            className="set-dirty-dot"
                            title="Unsaved changes"
                            aria-hidden="true"
                          />
                          <span className="visually-hidden">
                            , unsaved changes
                          </span>
                        </>
                      )}
                    </button>
                  );
                })}
              </div>
            ))}
          </div>
          <div className="set-mobile-nav">
            <Select
              aria-label="Settings section"
              value={active}
              onChange={(event) => setActive(event.target.value as never)}
            >
              {sections.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.title}
                  {(dirty[s.id] ?? 0) > 0 ? " •" : ""}
                </option>
              ))}
            </Select>
          </div>
          <URLPrivacyToggle />
        </aside>
        <div className="set-content">
          {sections.map((s) => (
            <div
              key={s.id}
              role="tabpanel"
              id={`${id}-${s.id}-panel`}
              aria-labelledby={`${id}-${s.id}-tab`}
              tabIndex={0}
              hidden={active !== s.id}
            >
              <SectionContext.Provider
                value={{ id: s.id, title: s.title, report }}
              >
                {(visited.has(s.id) || active === s.id) && panels[s.id]}
              </SectionContext.Provider>
            </div>
          ))}
        </div>
      </div>
    </URLPrivacyProvider>
  );
}
