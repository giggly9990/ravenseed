/** Relative times and durations shared by every page. */

const units: [Intl.RelativeTimeFormatUnit, number][] = [
  ["year", 365 * 24 * 3600],
  ["month", 30 * 24 * 3600],
  ["week", 7 * 24 * 3600],
  ["day", 24 * 3600],
  ["hour", 3600],
  ["minute", 60],
  ["second", 1],
];
const format = new Intl.RelativeTimeFormat("en", { numeric: "auto" });

/** "12 seconds ago", "3 hours ago", "yesterday". */
export function relativeTime(value: string, now = Date.now()) {
  const time = new Date(value).getTime();
  if (Number.isNaN(time)) return "";
  // Small clock skew between server and browser should not read "in 2 minutes".
  const seconds = Math.min(0, Math.round((time - now) / 1000));
  if (seconds > -5) return "just now";
  for (const [unit, size] of units)
    if (Math.abs(seconds) >= size || unit === "second")
      return format.format(Math.round(seconds / size), unit);
  return "";
}

/** "4.2 s", "1 min 12 s", "1 h 3 min". */
export function duration(start: string, end?: string) {
  if (!end) return "";
  const ms = new Date(end).getTime() - new Date(start).getTime();
  if (!Number.isFinite(ms) || ms < 0) return "";
  const s = ms / 1000;
  if (s < 60) return `${s.toFixed(1)} s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m} min ${Math.round(s % 60)} s`;
  return `${Math.floor(m / 60)} h ${m % 60} min`;
}

/** "just now", "4 min ago", "5 h ago", "6 days ago"; future times read "in 3 h". */
export function shortRelativeTime(value: string | undefined, now = Date.now()) {
  if (!value) return "";
  const time = new Date(value).getTime();
  if (Number.isNaN(time)) return "";
  const diff = now - time;
  const future = diff < 0;
  const minutes = Math.round(Math.abs(diff) / 60_000);
  let text: string;
  if (minutes < 1) return future ? "now" : "just now";
  if (minutes < 60) text = `${minutes} min`;
  else if (minutes < 60 * 36) text = `${Math.round(minutes / 60)} h`;
  else {
    const days = Math.round(minutes / (60 * 24));
    text = `${days} ${days === 1 ? "day" : "days"}`;
  }
  return future ? `in ${text}` : `${text} ago`;
}
