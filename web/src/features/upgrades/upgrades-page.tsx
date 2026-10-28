import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { api, apiFailure, bytes, safeURL } from "../../api";
import type { Monitor, MonitorCandidate, Page, Result } from "../../types";
import { Button } from "../../components/ui/button";
import { Pager } from "../../components/ui/pager";
import { Select } from "../../components/ui/select";
import { AddTorrent } from "../../components/add-torrent";
import { ExternalLinkIcon, SearchIcon } from "../../components/icons";
import { ErrorNotice } from "../../app/shared";
import { shortRelativeTime } from "../../lib/time";

const PAGE = 50;
type Level = "" | "upgrade" | "possible";

const levels: { value: Level; label: string }[] = [
  { value: "", label: "All" },
  { value: "upgrade", label: "Upgrades" },
  { value: "possible", label: "Possible" },
];

/** Readable reason, e.g. "correction: original → REPACK". */
function reasons(c: MonitorCandidate) {
  return (c.upgradeOffer?.upgrade ?? []).map((u) => {
    const kind = u.type.replace(/_/g, " ");
    return `${kind}: ${u.from} → ${u.to}`;
  });
}

export function UpgradesPage() {
  const qc = useQueryClient();
  const [level, setLevel] = useState<Level>("");
  const [monitorId, setMonitorId] = useState("");
  const [dismissed, setDismissed] = useState(false);
  const [offset, setOffset] = useState(0);
  const [text, setText] = useState("");
  const [search, setSearch] = useState("");
  useEffect(() => {
    const next = text.trim();
    if (next === search) return;
    const timer = setTimeout(() => {
      setSearch(next);
      setOffset(0);
    }, 300);
    return () => clearTimeout(timer);
  }, [text, search]);
  const monitors = useQuery({
    queryKey: ["monitors"],
    queryFn: () => api<Monitor[]>("/monitors"),
  });
  const upgrades = useQuery({
    queryKey: ["upgrades", level, monitorId, dismissed, search, offset],
    queryFn: () => {
      const query = new URLSearchParams({
        limit: String(PAGE),
        offset: String(offset),
      });
      if (level) query.set("level", level);
      if (monitorId) query.set("monitorId", monitorId);
      if (dismissed) query.set("dismissed", "true");
      if (search) query.set("q", search);
      return api<Page<MonitorCandidate>>(`/upgrades?${query}`);
    },
    refetchInterval: 30000,
  });
  const dismiss = useMutation({
    mutationFn: ({ id, value }: { id: string; value: boolean }) =>
      api(`/monitor-candidates/${id}/upgrade/dismiss`, {
        method: "POST",
        body: JSON.stringify({ dismissed: value }),
      }),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["upgrades"] }),
    // The target's release changed since this list loaded: show the new offer.
    onError: (error) => {
      if (apiFailure(error).code === "upgrade_changed")
        void qc.invalidateQueries({ queryKey: ["upgrades"] });
    },
  });
  const monitorFor = (id: string) => monitors.data?.find((m) => m.id === id);
  const items = upgrades.data?.items ?? [];
  const filtered = !!(level || monitorId || search);
  return (
    <>
      <header className="page-header">
        <h1>Upgrades</h1>
        <p>
          Torrents you seed whose slot on the monitored tracker now holds a
          different, possibly better release. Found by tracker monitor checks.
        </p>
      </header>

      <section className="panel upg-panel" aria-label="Upgrades">
        <div className="upg-toolbar">
          <div className="chips" role="group" aria-label="Upgrade level">
            {levels.map((l) => (
              <button
                key={l.value || "all"}
                type="button"
                className="chip"
                aria-pressed={level === l.value}
                onClick={() => {
                  setLevel(l.value);
                  setOffset(0);
                }}
              >
                {l.label}
              </button>
            ))}
          </div>
          <Select
            aria-label="Monitor"
            value={monitorId}
            onChange={(e) => {
              setMonitorId(e.target.value);
              setOffset(0);
            }}
          >
            <option value="">All monitors</option>
            {monitors.data?.map((m) => (
              <option key={m.id} value={m.id}>
                {m.sourceIndexerName} → {m.targetIndexerName}
              </option>
            ))}
          </Select>
          <label className="mon-search">
            <SearchIcon size={14} aria-hidden="true" />
            <input
              aria-label="Filter by name"
              placeholder="Name or group"
              value={text}
              onChange={(e) => setText(e.target.value)}
            />
          </label>
          <label className="check-row small">
            <input
              type="checkbox"
              checked={dismissed}
              onChange={(e) => {
                setDismissed(e.target.checked);
                setOffset(0);
              }}
            />
            Show dismissed
          </label>
        </div>
        <ErrorNotice
          error={
            upgrades.error ??
            (apiFailure(dismiss.error).code === "upgrade_changed"
              ? "This upgrade changed while you were looking; the list now shows the current offer."
              : dismiss.error)
          }
        />
        {upgrades.data && items.length === 0 ? (
          <p className="table-empty">
            {dismissed
              ? "No dismissed upgrades."
              : filtered
                ? "Nothing matches these filters."
                : "No upgrades found yet. Monitors with an indexer policy report them as they check torrents."}
          </p>
        ) : (
          <div className="table-wrap">
            <table className="data-table">
              <thead>
                <tr>
                  <th scope="col">Your torrent</th>
                  <th scope="col">On the target</th>
                  <th scope="col">Checked</th>
                  <th scope="col" className="actions">
                    <span className="visually-hidden">Actions</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {items.map((c) => {
                  const offer = c.upgradeOffer!;
                  const m = monitorFor(c.monitorId);
                  const why = reasons(c);
                  const link = offer.infoUrl && safeURL(offer.infoUrl);
                  const release: Result = {
                    id: offer.id,
                    indexerId: m?.targetIndexerId ?? 0,
                    indexer: offer.indexer || m?.targetIndexerName || "",
                    title: offer.title,
                    size: offer.size,
                  } as Result;
                  return (
                    <tr key={c.id}>
                      <td className="upg-cell">
                        <span className="cell-title break mono">{c.name}</span>
                        <span className="cell-sub">
                          {bytes(c.size)}
                          {m && ` · ${m.sourceIndexerName}`}
                        </span>
                      </td>
                      <td className="upg-cell">
                        <span
                          className={`pill ${c.upgrade === "upgrade" ? "accent" : "warn"}`}
                        >
                          {c.upgrade === "upgrade"
                            ? "Upgrade"
                            : "Possible upgrade"}
                        </span>{" "}
                        <span className="cell-title break mono">
                          {offer.title}
                        </span>
                        <span className="cell-sub">
                          {bytes(offer.size)}
                          {m && ` · ${m.targetIndexerName}`}
                          {why.length > 0
                            ? ` · ${why.join("; ")}`
                            : " · Different group in your slot; compare before replacing"}
                          {offer.upgradeReview && ` · ${offer.upgradeReview}`}
                        </span>
                      </td>
                      <td className="small faint">
                        {shortRelativeTime(c.lastCheckedAt) || "–"}
                      </td>
                      <td className="actions upg-actions">
                        {link && (
                          <a
                            className="button sm quiet"
                            href={link}
                            target="_blank"
                            rel="noreferrer noopener"
                          >
                            <ExternalLinkIcon size={13} />
                            Open
                            <span className="visually-hidden">
                              {" "}
                              {offer.title} on the target tracker (opens in a
                              new tab)
                            </span>
                          </a>
                        )}
                        <AddTorrent
                          result={release}
                          addPath={`/monitor-candidates/${c.id}/upgrade/add`}
                          allowUpload={false}
                          available
                        />
                        <Button
                          className="sm quiet"
                          disabled={dismiss.isPending}
                          onClick={() =>
                            dismiss.mutate({ id: c.id, value: !dismissed })
                          }
                        >
                          {dismissed ? "Restore" : "Dismiss"}
                        </Button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
        <Pager
          offset={offset}
          total={upgrades.data?.total}
          page={PAGE}
          onChange={setOffset}
          buttonClassName="sm"
        />
        <p className="hint">
          Adding downloads the target's release through qui and leaves your
          current torrent untouched. A dismissed upgrade returns if the target's
          release changes. <Link to="/monitors">Monitors</Link> find upgrades
          when the target tracker has an enabled indexer policy.
        </p>
      </section>
    </>
  );
}
