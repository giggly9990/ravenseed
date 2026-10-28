import { useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api";
import type { Indexer, Monitor, TrackerHost } from "../../types";
import { Button } from "../../components/ui/button";
import { Select } from "../../components/ui/select";
import { Dialog } from "../../components/ui/dialog";
import { ErrorNotice } from "../../app/shared";
import { InstancePicker, useAvailableInstances } from "./instance-picker";

export function CreateMonitorDialog({
  close,
  created,
}: {
  close: () => void;
  created: (monitor: Monitor) => void;
}) {
  const qc = useQueryClient();
  const [source, setSource] = useState(0);
  const [target, setTarget] = useState(0);
  const [host, setHost] = useState("");
  const [hours, setHours] = useState(24);
  const [chosen, setChosen] = useState<number[]>();
  const indexers = useQuery({
    queryKey: ["indexers"],
    queryFn: () => api<Indexer[]>("/indexers"),
  });
  const { instances, available, pick } = useAvailableInstances();
  const hosts = useQuery({
    queryKey: ["tracker-hosts"],
    queryFn: () => api<TrackerHost[]>("/tracker-hosts"),
  });
  const create = useMutation({
    mutationFn: () =>
      api<Monitor>("/monitors", {
        method: "POST",
        body: JSON.stringify({
          sourceIndexerId: source,
          sourceTrackerHost: offeredHost,
          targetIndexerId: target,
          intervalHours: hours,
          instanceIds: picked,
        }),
      }),
    onSuccess: (monitor) => {
      void qc.invalidateQueries({ queryKey: ["monitors"] });
      created(monitor);
    },
  });
  function submit(event: FormEvent) {
    event.preventDefault();
    create.mutate();
  }
  const enabled = indexers.data?.filter((i) => i.enabled) ?? [];
  // A lone available instance starts ticked.
  const picked = pick(
    chosen ?? (available.length === 1 ? [available[0].id] : []),
  );
  // Hosts and counts cover only the chosen instances.
  const offered = (hosts.data ?? []).flatMap((h) => {
    const count = picked.reduce((n, id) => n + (h.instanceCounts[id] ?? 0), 0);
    return count ? [{ host: h.host, count }] : [];
  });
  const offeredHost = offered.some((h) => h.host === host) ? host : "";
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !create.isPending) close();
      }}
      title="New monitor"
      description="Tracker A's announce host tells RavenSeed which qui torrents to check against Tracker B."
    >
      <form onSubmit={submit} className="dialog-body">
        <InstancePicker
          available={available}
          loaded={instances.isSuccess}
          chosen={picked}
          toggle={(id, on) =>
            setChosen(on ? [...picked, id] : picked.filter((v) => v !== id))
          }
        />
        <div className="form-grid two">
          <label>
            Tracker A indexer
            <Select
              aria-label="Tracker A indexer"
              value={source || ""}
              onChange={(e) => setSource(+e.target.value)}
              required
            >
              <option value="">Choose source</option>
              {enabled.map((i) => (
                <option key={i.id} value={i.id}>
                  {i.name}
                </option>
              ))}
            </Select>
          </label>
          <label>
            Tracker A announce host
            <Select
              aria-label="Tracker A announce host"
              value={offeredHost}
              onChange={(e) => setHost(e.target.value)}
              required
            >
              <option value="">
                {picked.length
                  ? "Choose host found in qui"
                  : "Tick an instance to list its tracker hosts"}
              </option>
              {offered.map((h) => (
                <option key={h.host} value={h.host}>
                  {h.host} · {h.count} torrents
                </option>
              ))}
            </Select>
          </label>
          <label>
            Tracker B target indexer
            <Select
              aria-label="Tracker B target indexer"
              value={target || ""}
              onChange={(e) => setTarget(+e.target.value)}
              required
            >
              <option value="">Choose target</option>
              {enabled
                .filter((i) => i.id !== source)
                .map((i) => (
                  <option key={i.id} value={i.id}>
                    {i.name}
                  </option>
                ))}
            </Select>
          </label>
          <label>
            Recheck every (hours)
            <input
              aria-label="Recheck interval"
              type="number"
              min={6}
              max={168}
              value={hours}
              onChange={(e) => setHours(+e.target.value)}
              required
            />
          </label>
        </div>
        <ErrorNotice
          error={
            indexers.error || instances.error || hosts.error || create.error
          }
        />
        <div className="dialog-actions">
          <Button
            type="button"
            className="quiet"
            onClick={close}
            disabled={create.isPending}
          >
            Cancel
          </Button>
          <Button
            className="primary"
            type="submit"
            disabled={
              create.isPending ||
              !picked.length ||
              !source ||
              !offeredHost ||
              !target ||
              source === target
            }
          >
            Create monitor
          </Button>
        </div>
      </form>
    </Dialog>
  );
}
