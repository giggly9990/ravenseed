import { useState } from "react";
import * as Menu from "@radix-ui/react-dropdown-menu";
import type { Monitor, QuiInstance } from "../../types";
import type { CandidateClass } from "../../hooks/use-candidate-counts";
import { uploadableClasses } from "../../hooks/use-candidate-counts";
import { Button } from "../../components/ui/button";
import { Dialog } from "../../components/ui/dialog";
import { MoreIcon, RefreshIcon } from "../../components/icons";
import { ErrorNotice } from "../../app/shared";
import { shortRelativeTime } from "../../lib/time";
import { EditInstancesDialog } from "./edit-instances-dialog";

export function MonitorCard({
  monitor: m,
  instances,
  counts,
  selected,
  select,
  scan,
  scanning,
  toggle,
  toggling,
  remove,
  removing,
  cardRef,
}: {
  monitor: Monitor;
  /** qui instances, to name the monitor's scope. */
  instances?: QuiInstance[];
  counts: Partial<Record<CandidateClass, number>>;
  selected: boolean;
  select: () => void;
  scan: () => void;
  scanning: boolean;
  toggle: () => void;
  toggling: boolean;
  remove: () => void;
  removing: boolean;
  cardRef?: (node: HTMLDivElement | null) => void;
}) {
  const [confirming, setConfirming] = useState(false);
  const [editing, setEditing] = useState(false);
  const scope = m.instanceIds?.length
    ? m.instanceIds
        .map((id) => instances?.find((i) => i.id === id)?.name ?? `#${id}`)
        .join(", ")
    : "all instances";
  const pair = `${m.sourceIndexerName} → ${m.targetIndexerName}`;
  const state = m.lastError ? "offline" : m.enabled ? "online" : "paused";
  const known = uploadableClasses.some((c) => counts[c] !== undefined);
  const toUpload = uploadableClasses.reduce((n, c) => n + (counts[c] ?? 0), 0);
  const show = (value: number | undefined) =>
    value === undefined ? "–" : value.toLocaleString();
  return (
    <div
      role="listitem"
      ref={cardRef}
      className={`mon-card${selected ? " selected" : ""}`}
    >
      <div className="mon-card-head">
        <span
          className={`status-dot ${state}`}
          title={
            m.lastError ? "Last scan failed" : m.enabled ? "Active" : "Paused"
          }
        />
        <button
          type="button"
          className="mon-card-select"
          aria-pressed={selected}
          onClick={select}
        >
          {pair}
        </button>
        {!m.enabled && <span className="pill">Paused</span>}
        <span className="spacer" />
        <button
          type="button"
          className="icon-button"
          aria-label="Scan qui now"
          title="Scan qui now"
          onClick={scan}
          disabled={scanning || !m.enabled}
        >
          <RefreshIcon size={15} className={scanning ? "spin" : undefined} />
        </button>
        <Menu.Root modal={false}>
          <Menu.Trigger asChild>
            <button
              type="button"
              className="icon-button"
              aria-label="Monitor options"
            >
              <MoreIcon size={15} />
            </button>
          </Menu.Trigger>
          <Menu.Portal>
            <Menu.Content className="menu" align="end" sideOffset={4}>
              <Menu.Item
                className="menu-item"
                disabled={toggling}
                onSelect={toggle}
              >
                {m.enabled ? "Pause rechecks" : "Resume rechecks"}
              </Menu.Item>
              <Menu.Item
                className="menu-item"
                onSelect={() => setEditing(true)}
              >
                Edit instances
              </Menu.Item>
              <Menu.Separator className="menu-separator" />
              <Menu.Item
                className="menu-item danger"
                disabled={removing}
                onSelect={() => setConfirming(true)}
              >
                Delete
              </Menu.Item>
            </Menu.Content>
          </Menu.Portal>
        </Menu.Root>
      </div>
      <div className="mon-card-meta mono">
        {m.sourceTrackerHost} · {scope} · every {m.intervalHours} h ·{" "}
        {m.lastScanAt
          ? `scanned ${shortRelativeTime(m.lastScanAt)}`
          : "not scanned yet"}
      </div>
      <div className="mon-card-counts">
        <span>
          <strong className="num">{show(counts.all)}</strong> torrents
        </span>
        <span className="hot">
          <strong className="num">{known ? show(toUpload) : "–"}</strong> to
          upload
        </span>
        <span className="review">
          <strong className="num">{show(counts.possible_trump)}</strong> review
        </span>
      </div>
      {m.lastError && (
        <div className="mon-card-error" title={m.lastError}>
          <ErrorNotice error={m.lastError} />
        </div>
      )}
      {editing && (
        <EditInstancesDialog monitor={m} close={() => setEditing(false)} />
      )}
      {confirming && (
        <Dialog
          open
          onOpenChange={(open) => {
            if (!open) setConfirming(false);
          }}
          title="Delete monitor?"
          description={`${pair} and its candidate history will be removed. Torrents in qui are not touched.`}
        >
          <div className="dialog-actions">
            <Button className="quiet" onClick={() => setConfirming(false)}>
              Cancel
            </Button>
            <Button
              className="danger"
              disabled={removing}
              onClick={() => {
                remove();
                setConfirming(false);
              }}
            >
              Delete monitor
            </Button>
          </div>
        </Dialog>
      )}
    </div>
  );
}
