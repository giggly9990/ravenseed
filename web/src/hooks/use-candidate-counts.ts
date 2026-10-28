import { useEffect, useRef } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "../api";
import type { Monitor } from "../types";

/** Classification filters the candidates endpoint understands. */
export const candidateClasses = [
  "all",
  "missing_slot",
  "trump",
  "possible_trump",
  "coexistence",
  "equivalent",
  "not_eligible",
  "queued",
  "errors",
  "gone",
] as const;
export type CandidateClass = (typeof candidateClasses)[number];

/** Classifications that are worth sending to Upload-Assistant. */
export const uploadableClasses: CandidateClass[] = [
  "missing_slot",
  "trump",
  "coexistence",
];

export type CandidateCountRow = Partial<Record<CandidateClass, number>>;

/** Shared by every caller; invalidate it after a candidate changes. */
export const candidateCountKey = ["candidate-count"] as const;

/**
 * Per-classification totals for each monitor, from one request that counts
 * every monitor (the same totals the candidates list reports per filter).
 * Callers share the query; the shortest refetchInterval among them wins.
 */
export function useCandidateCounts(
  monitors: Pick<Monitor, "id">[] | undefined,
  classes: readonly CandidateClass[] = candidateClasses,
  refetchInterval = 30_000,
) {
  const list = monitors ?? [];
  const query = useQuery({
    queryKey: candidateCountKey,
    queryFn: () =>
      api<Record<string, Record<string, number>>>("/monitor-candidate-counts"),
    enabled: list.length > 0,
    refetchInterval,
    staleTime: 10_000,
  });
  // A monitor created since the last count is not in it yet: count again,
  // once per set of missing monitors.
  const missing = query.data
    ? list
        .filter((m) => !(m.id in query.data))
        .map((m) => m.id)
        .join(",")
    : "";
  const requested = useRef("");
  const { refetch } = query;
  useEffect(() => {
    if (!missing || missing === requested.current) return;
    requested.current = missing;
    void refetch();
  }, [missing, refetch]);
  const counts: Record<string, CandidateCountRow> = {};
  for (const m of list) {
    const row: CandidateCountRow = {};
    const totals = query.data?.[m.id];
    if (totals)
      for (const c of classes)
        if (typeof totals[c] === "number") row[c] = totals[c];
    counts[m.id] = row;
  }
  return counts;
}

export function uploadableCount(row: CandidateCountRow) {
  return uploadableClasses.reduce((sum, c) => sum + (row[c] ?? 0), 0);
}
