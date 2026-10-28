import type { UploadSettings } from "../types";
import { CloseIcon } from "./icons";

/**
 * A new preset ID matching the server's `[a-zA-Z0-9_-]{1,80}`.
 * crypto.randomUUID() only exists in secure contexts (HTTPS or localhost) and
 * throws on plain-http LAN addresses; getRandomValues works everywhere.
 */
export function newPresetId(taken: readonly string[] = []) {
  for (;;) {
    const bytes = new Uint8Array(12);
    if (typeof globalThis.crypto?.getRandomValues === "function")
      crypto.getRandomValues(bytes);
    else for (let i = 0; i < bytes.length; i++) bytes[i] = Math.random() * 256;
    const id = Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join(
      "",
    );
    if (!taken.includes(id)) return id;
  }
}

/** Flag presets table: default radio · name · UA flags · remove. */
export function UploadPresets({
  value,
  onChange,
}: {
  value: UploadSettings;
  onChange: (v: UploadSettings) => void;
}) {
  const presets = value.flagPresets ?? [];
  const name = (i: number) => presets[i].name.trim() || `preset ${i + 1}`;
  return (
    <div className="panel">
      <div className="panel-head">
        <h3>Flag presets</h3>
        <span className="hint">
          RavenSeed adds the tracker, media IDs and unattended mode for you.
          Quote values containing spaces.
        </span>
        {value.defaultFlagPresetId && (
          <div className="actions">
            <button
              type="button"
              className="link-button small"
              onClick={() => onChange({ ...value, defaultFlagPresetId: "" })}
            >
              Clear default
            </button>
          </div>
        )}
      </div>
      {presets.length > 0 && (
        <div className="table-wrap">
          <table className="data-table compact set-edit-table">
            <thead>
              <tr>
                <th className="set-col-radio">Default</th>
                <th className="set-col-name">Name</th>
                <th>UA flags</th>
                <th className="set-col-remove">
                  <span className="visually-hidden">Remove</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {presets.map((p, i) => (
                <tr key={p.id}>
                  <td className="set-col-radio">
                    <input
                      type="radio"
                      name="ua-default-preset"
                      aria-label={`Use ${name(i)} as default`}
                      checked={value.defaultFlagPresetId === p.id}
                      onChange={() =>
                        onChange({ ...value, defaultFlagPresetId: p.id })
                      }
                    />
                  </td>
                  <td>
                    <input
                      required
                      maxLength={80}
                      aria-label={`Preset name ${i + 1}`}
                      placeholder="Name"
                      value={p.name}
                      onChange={(e) =>
                        onChange({
                          ...value,
                          flagPresets: presets.map((v) =>
                            v.id === p.id ? { ...v, name: e.target.value } : v,
                          ),
                        })
                      }
                    />
                  </td>
                  <td>
                    <input
                      className="mono"
                      maxLength={4096}
                      aria-label={`UA flags for ${name(i)}`}
                      value={p.args}
                      placeholder="--anon --modq"
                      spellCheck={false}
                      onChange={(e) =>
                        onChange({
                          ...value,
                          flagPresets: presets.map((v) =>
                            v.id === p.id ? { ...v, args: e.target.value } : v,
                          ),
                        })
                      }
                    />
                  </td>
                  <td className="set-col-remove">
                    <button
                      type="button"
                      className="icon-button"
                      aria-label={`Remove ${name(i)}`}
                      onClick={() =>
                        onChange({
                          ...value,
                          flagPresets: presets.filter((v) => v.id !== p.id),
                          defaultFlagPresetId:
                            value.defaultFlagPresetId === p.id
                              ? ""
                              : value.defaultFlagPresetId,
                        })
                      }
                    >
                      <CloseIcon size={14} />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <button
        type="button"
        className="link-button set-add"
        disabled={presets.length >= 100}
        onClick={() =>
          onChange({
            ...value,
            flagPresets: [
              ...presets,
              {
                id: newPresetId(presets.map((p) => p.id)),
                name: "",
                args: "",
              },
            ],
          })
        }
      >
        + Add preset
      </button>
    </div>
  );
}
