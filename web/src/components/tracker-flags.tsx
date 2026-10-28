import type { Result } from "../types";

export function trackerFlags(
  result: Pick<Result, "raw" | "downloadFactor" | "uploadFactor">,
): string[] {
  const raw =
    result.raw && typeof result.raw === "object"
      ? (result.raw as Record<string, unknown>)
      : null;
  const download = raw ? raw.download_volume_factor : result.downloadFactor;
  const upload = raw ? raw.upload_volume_factor : result.uploadFactor;
  const flags: string[] = [];
  if (
    typeof download === "number" &&
    Number.isFinite(download) &&
    download >= 0
  ) {
    if (download === 0) flags.push("Freeleech");
    else if (download !== 1)
      flags.push(`${Number((download * 100).toFixed(2))}% download`);
  }
  if (
    typeof upload === "number" &&
    Number.isFinite(upload) &&
    upload >= 0 &&
    upload !== 1
  ) {
    flags.push(upload === 0 ? "No upload credit" : `${upload}× upload`);
  }
  return flags;
}

/** A short form of a flag for dense chips, e.g. "FL", "2×↑", "50%". */
export function shortFlag(flag: string): string {
  if (flag === "Freeleech") return "FL";
  if (flag === "No upload credit") return "0×↑";
  const download = /^([\d.]+)% download$/.exec(flag);
  if (download) return `${download[1]}%`;
  const upload = /^([\d.]+)× upload$/.exec(flag);
  if (upload) return `${upload[1]}×↑`;
  return flag;
}

export function TrackerFlags({
  results,
  short = false,
}: {
  results: Result[];
  /** Dense form for table chips; the full wording stays in the tooltip. */
  short?: boolean;
}) {
  const sets = results.map(trackerFlags);
  const varied = sets.some((set) => set.join("|") !== sets[0]?.join("|"));
  if (short) {
    const flags = varied ? ["Mixed flags"] : (sets[0] ?? []);
    if (!flags.length) return null;
    const full = varied
      ? "Flags differ between listings. Open details to compare."
      : flags.join(", ");
    return (
      <span className="tracker-flags short" title={full}>
        <span aria-hidden="true">
          {varied ? "mixed" : flags.map(shortFlag).join(" ")}
        </span>
        <span className="visually-hidden">{full}</span>
      </span>
    );
  }
  return (
    <span className="tracker-flags">
      {varied ? (
        <span
          className="flag-badge"
          title="Flags differ between listings. Open details to compare."
        >
          Mixed flags
        </span>
      ) : (
        sets[0]?.map((flag) => (
          <span className="flag-badge" key={flag}>
            {flag}
          </span>
        ))
      )}
    </span>
  );
}
