import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api";
import type { Monitor } from "../../types";
import { candidateCountKey } from "../../hooks/use-candidate-counts";
import { Button } from "../../components/ui/button";
import { Dialog } from "../../components/ui/dialog";
import { ErrorNotice } from "../../app/shared";
import { InstancePicker, useAvailableInstances } from "./instance-picker";

export function EditInstancesDialog({
  monitor: m,
  close,
}: {
  monitor: Monitor;
  close: () => void;
}) {
  const qc = useQueryClient();
  const { instances, available, pick } = useAvailableInstances();
  // A monitor saved before instance selection, or saved with all
  // instances, has no list: it scans every instance, including new ones.
  // Read once, like the ticks, so a refetch while open changes neither.
  const [kept] = useState(() => m.instanceIds ?? []);
  const [all, setAll] = useState(!kept.length);
  const [chosen, setChosen] = useState(kept.length ? kept : undefined);
  // Instances already on the monitor stay while offline; only online ones
  // can be added.
  const picked = pick(chosen ?? available.map((i) => i.id), kept);
  const offline = instances.isSuccess
    ? kept
        .filter((id) => !available.some((i) => i.id === id))
        .map((id) => ({
          id,
          name: instances.data.find((i) => i.id === id)?.name ?? `#${id}`,
        }))
    : [];
  const unchanged = all
    ? !kept.length
    : picked.length === kept.length && picked.every((id) => kept.includes(id));
  const save = useMutation({
    mutationFn: () =>
      api<Monitor>(`/monitors/${m.id}`, {
        method: "PUT",
        body: JSON.stringify({ instanceIds: all ? [] : picked }),
      }),
    onSuccess: (updated) => {
      qc.setQueryData<Monitor[]>(["monitors"], (items) =>
        items?.map((item) => (item.id === updated.id ? updated : item)),
      );
      void qc.invalidateQueries({ queryKey: ["monitors"] });
      void qc.invalidateQueries({ queryKey: ["monitor-candidates"] });
      void qc.invalidateQueries({ queryKey: candidateCountKey });
      close();
    },
    // A rejected instance usually went offline: show the current list.
    onError: () => void qc.invalidateQueries({ queryKey: ["qui-instances"] }),
  });
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !save.isPending) close();
      }}
      title="Edit instances"
      description={`${m.enabled ? "Added instances are scanned at once. " : ""}Torrents on removed instances leave the monitor over their next checks.`}
    >
      <div className="dialog-body">
        <label className="check-row">
          <input
            type="checkbox"
            checked={all}
            onChange={(e) => setAll(e.target.checked)}
            disabled={save.isPending}
          />
          All instances, including ones added later
        </label>
        {!all && (
          <InstancePicker
            available={available}
            offline={offline}
            loaded={instances.isSuccess}
            chosen={picked}
            toggle={(id, on) =>
              setChosen(on ? [...picked, id] : picked.filter((v) => v !== id))
            }
            disabled={save.isPending}
          />
        )}
        {!all && instances.isSuccess && !picked.length && (
          <p className="muted">
            Tick at least one instance, or choose all instances.
          </p>
        )}
        <ErrorNotice error={instances.error || save.error} />
        <div className="dialog-actions">
          <Button
            type="button"
            className="quiet"
            onClick={close}
            disabled={save.isPending}
          >
            Cancel
          </Button>
          <Button
            className="primary"
            disabled={save.isPending || unchanged || (!all && !picked.length)}
            onClick={() => save.mutate()}
          >
            Save instances
          </Button>
        </div>
      </div>
    </Dialog>
  );
}
