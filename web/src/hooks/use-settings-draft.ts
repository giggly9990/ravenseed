import { useEffect, useState } from "react";

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

/**
 * Number of differing fields between two JSON-like values. Objects are
 * compared key by key (recursively), arrays element by element; anything
 * else counts as one change when not identical. `undefined`, `""`, `false`
 * and missing keys are not treated as equal: callers normalise their values.
 */
export function countChanges(a: unknown, b: unknown): number {
  if (Object.is(a, b)) return 0;
  if (Array.isArray(a) && Array.isArray(b)) {
    let n = Math.abs(a.length - b.length);
    for (let i = 0; i < Math.min(a.length, b.length); i++)
      if (countChanges(a[i], b[i])) n++;
    return n;
  }
  if (isRecord(a) && isRecord(b)) {
    let n = 0;
    for (const key of new Set([...Object.keys(a), ...Object.keys(b)])) {
      if (a[key] === undefined && b[key] === undefined) continue;
      n += countChanges(a[key], b[key]);
    }
    return n;
  }
  return 1;
}

/**
 * Local edit buffer for a settings section. Refetches replace the draft only
 * while it has no unsaved changes (including edits the user reverted), so
 * saving one section never clobbers another section's edits.
 * `changes`/`dirty` compare the draft with the last saved value; `reset()`
 * discards edits.
 */
export function useSettingsDraft<T>(saved: T | undefined, initial: T) {
  const [draft, setDraft] = useState(saved ?? initial);
  const [baseline, setBaseline] = useState(saved ?? initial);
  const changes = countChanges(draft, baseline);
  useEffect(() => {
    if (saved === undefined) return;
    setBaseline(saved);
    if (changes === 0) setDraft(saved);
    // Only a new saved value syncs; `changes` is read as of that render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [saved]);
  const accept = (value: T) => {
    setBaseline(value);
    setDraft(value);
  };
  const reset = () => setDraft(baseline);
  return [
    draft,
    setDraft,
    accept,
    { changes, dirty: changes > 0, reset, saved: baseline },
  ] as const;
}
