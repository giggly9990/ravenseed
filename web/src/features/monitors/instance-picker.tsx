import { useQuery } from "@tanstack/react-query";
import { api } from "../../api";
import type { QuiInstance } from "../../types";

/** qui instances, fetched fresh when a picker opens; only active,
 * connected ones are available to a monitor. */
export function useAvailableInstances() {
  const instances = useQuery({
    queryKey: ["qui-instances"],
    queryFn: () => api<QuiInstance[]>("/qui/instances"),
    staleTime: 0,
  });
  const available =
    instances.data?.filter((i) => i.isActive && i.connected) ?? [];
  /** The selection without instances that went offline: those have no
   * checkbox to untick, and the server rejects them. `kept` instances
   * (already on the monitor) stay even while offline. */
  const pick = (chosen: number[], kept: number[] = []) =>
    chosen.filter(
      (id) => kept.includes(id) || available.some((i) => i.id === id),
    );
  return { instances, available, pick };
}

export function InstancePicker({
  available,
  offline = [],
  loaded,
  chosen,
  toggle,
  disabled,
}: {
  available: QuiInstance[];
  /** Instances already on the monitor that are offline now: they can be
   * kept or unticked, but not newly added. */
  offline?: { id: number; name: string }[];
  loaded: boolean;
  chosen: number[];
  toggle: (id: number, on: boolean) => void;
  disabled?: boolean;
}) {
  return (
    <fieldset className="mon-instances">
      <legend>qui instances</legend>
      {loaded && !available.length && !offline.length && (
        <p className="muted">No active, connected qui instance.</p>
      )}
      {[...available, ...offline].map((i) => (
        <label key={i.id} className="check-row">
          <input
            type="checkbox"
            checked={chosen.includes(i.id)}
            onChange={(e) => toggle(i.id, e.target.checked)}
            disabled={disabled}
          />
          {i.name}
          {offline.some((o) => o.id === i.id) && (
            <>
              {" "}
              <span className="muted">offline</span>
            </>
          )}
        </label>
      ))}
    </fieldset>
  );
}
