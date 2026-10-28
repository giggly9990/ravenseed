import type { IdentitySource, Media, MonitorCandidate } from "../../types";
import type { CandidateClass } from "../../hooks/use-candidate-counts";

export const classLabels: Record<CandidateClass, string> = {
  all: "All",
  missing_slot: "Missing slot",
  trump: "Trump",
  possible_trump: "Needs review",
  coexistence: "Coexistence",
  equivalent: "Equivalent",
  not_eligible: "Occupied",
  queued: "Queued",
  errors: "Errors",
  gone: "Gone",
};

type Pill = { label: string; tone: string };

const policyPills: Record<string, Pill> = {
  missing_slot: { label: "Missing slot", tone: "accent" },
  trump: { label: "Trump", tone: "accent" },
  possible_trump: { label: "Needs review", tone: "warn" },
  coexistence: { label: "Coexistence", tone: "warn" },
  equivalent: { label: "Equivalent", tone: "ok" },
  not_eligible: { label: "Occupied", tone: "quiet" },
};

const statusPills: Record<string, Pill> = {
  checking: { label: "Checking…", tone: "dashed" },
  queued: { label: "Queued", tone: "dashed" },
  error: { label: "Error", tone: "danger" },
  gone: { label: "Gone", tone: "quiet" },
  candidate: { label: "Candidate", tone: "accent" },
  uncertain: { label: "Needs review", tone: "warn" },
  present: { label: "Equivalent", tone: "ok" },
  occupied: { label: "Occupied", tone: "quiet" },
  not_eligible: { label: "Occupied", tone: "quiet" },
};

/** Live states (checking, queued, error, gone) win over a stale policy. */
const liveStatuses = new Set(["checking", "queued", "error", "gone"]);

export function candidatePill(c: MonitorCandidate): Pill {
  if (!liveStatuses.has(c.status) && c.policy) {
    const pill = policyPills[c.policy.classification];
    if (pill) return pill;
  }
  return (
    statusPills[c.status] ?? {
      label: c.status.replaceAll("_", " ") || "Unknown",
      tone: "quiet",
    }
  );
}

/** Same rule the backend applies before it accepts an upload. */
export function canSendToUA(c: MonitorCandidate) {
  return (
    (c.status === "candidate" ||
      (c.status === "uncertain" &&
        c.policy?.classification === "possible_trump")) &&
    !c.uploadJobId
  );
}

export function parsedTags(c: MonitorCandidate) {
  const p = c.parsed;
  if (!p) return [];
  const tags = [
    p.resolution,
    p.source?.replaceAll("_", "-"),
    p.releaseType?.replaceAll("_", " "),
    p.videoCodec,
    p.bitDepth ? `${p.bitDepth}-bit` : "",
    ...(p.hdr ?? []),
    [p.audioCodec, p.audioChannels].filter(Boolean).join(" "),
    ...(p.edition ?? []),
    ...(p.cut ?? []),
    p.service ?? "",
    p.repack ? "REPACK" : "",
    p.proper ? "PROPER" : "",
    p.hybrid ? "Hybrid" : "",
    p.displayGroup || p.group,
  ];
  const seen = new Set<string>();
  return tags.filter((t): t is string => {
    if (!t || t === "OTHER") return false;
    const key = t.toLowerCase().replace(/[^a-z0-9+]/g, "");
    if (seen.has(key)) return false;
    seen.add(key);
    return true;
  });
}

export const identityLabels: Record<IdentitySource, string> = {
  arr_history: "download history",
  arr_parse: "library name match",
  arr_lookup: "title lookup",
  none: "no match",
};

/** "IMDb tt0133093 · TMDb 603 · TVDb 1" for the IDs present. */
export function mediaIDs(m: Media | undefined) {
  if (!m) return "";
  return [
    m.imdbId && `IMDb ${m.imdbId}`,
    m.tmdbId && `TMDb ${m.tmdbId}`,
    m.tvdbId && `TVDb ${m.tvdbId}`,
  ]
    .filter(Boolean)
    .join(" · ");
}

function usableIdentity(c: MonitorCandidate) {
  return !!c.identity && !c.identityError && !!mediaIDs(c.media);
}

/** IDs Upload-Assistant reuses without confirmation: download history only
 * (server rule). */
export function trustedIdentity(c: MonitorCandidate) {
  return usableIdentity(c) && c.identity?.source === "arr_history";
}

/** IDs from a Sonarr/Radarr library name match: prefilled, but sent only
 * after the user confirms them (server rule). */
export function nameMatchIdentity(c: MonitorCandidate) {
  return usableIdentity(c) && c.identity?.source === "arr_parse";
}
