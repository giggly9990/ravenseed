import {
  ErrorIcon,
  PendingIcon,
  SpinnerIcon,
  SuccessIcon,
} from "../components/icons";
import type { ReactNode } from "react";
import { RavenError } from "../components/raven-reactions";

export const label = (s: string) => s.replaceAll("_", " ");
export const date = (s: string) => new Date(s).toLocaleString();
/** Every resolution the UI filters on, highest first. */
export const resolutions = [
  "4320p",
  "2160p",
  "1440p",
  "1080p",
  "1080i",
  "720p",
  "576p",
  "480p",
];
/** Resolutions an indexer policy accepts (encode scope, bitrate tiers). */
export const policyResolutions = [
  "480p",
  "576p",
  "720p",
  "1080i",
  "1080p",
  "1440p",
  "2160p",
  "4320p",
];
const errorIds = new WeakMap<object, number>();
let nextErrorId = 0;
function errorKey(error: unknown) {
  if (typeof error !== "object" || error === null) return String(error);
  let id = errorIds.get(error);
  if (id === undefined) errorIds.set(error, (id = ++nextErrorId));
  return id;
}

/**
 * Shows an error with the xD raven. A new error object remounts the notice,
 * so the same message failing again escalates to xDD.
 */
export function ErrorNotice({
  error,
  action,
}: {
  error: unknown;
  action?: ReactNode;
}) {
  return error ? (
    <RavenError key={errorKey(error)} action={action}>
      {error instanceof Error ? error.message : String(error)}
    </RavenError>
  ) : null;
}
export function StatusIcon({ status }: { status: string }) {
  return status === "completed" ? (
    <SuccessIcon size={16} />
  ) : status === "failed" ? (
    <ErrorIcon size={16} />
  ) : status === "running" ? (
    <SpinnerIcon size={16} className="spin" />
  ) : (
    <PendingIcon size={16} />
  );
}
