import {
  createContext,
  useContext,
  useEffect,
  useId,
  useRef,
  useState,
  type InputHTMLAttributes,
  type ReactNode,
} from "react";
import { insecureApiUrl } from "../lib/insecure-url";
import { EyeIcon, EyeOffIcon } from "./icons";

const storageKey = "ravenseed.maskConnectionUrls";
const URLPrivacy = createContext({
  masked: false,
  setMasked: (_value: boolean) => {},
});

export function URLPrivacyProvider({ children }: { children: ReactNode }) {
  const [masked, setMaskedState] = useState(() => {
    try {
      return localStorage.getItem(storageKey) === "true";
    } catch {
      return false;
    }
  });
  const setMasked = (value: boolean) => {
    setMaskedState(value);
    try {
      localStorage.setItem(storageKey, String(value));
    } catch {}
  };
  return (
    <URLPrivacy.Provider value={{ masked, setMasked }}>
      {children}
    </URLPrivacy.Provider>
  );
}

export function URLPrivacyToggle() {
  const { masked, setMasked } = useContext(URLPrivacy);
  return (
    <label className="check-row set-mask-toggle">
      <input
        type="checkbox"
        checked={masked}
        onChange={(event) => setMasked(event.target.checked)}
      />
      Mask URLs on screen
    </label>
  );
}

// URLField edits a connection URL whose requests carry an API key, so it
// warns when that key would cross the internet over plain http.
export function URLField({
  label,
  value,
  ...props
}: Omit<InputHTMLAttributes<HTMLInputElement>, "type" | "value" | "id"> & {
  label: string;
  value: string;
}) {
  const { masked } = useContext(URLPrivacy);
  const [revealed, setRevealed] = useState(false);
  const input = useRef<HTMLInputElement>(null);
  const id = useId();
  const warningId = `${id}-insecure`;
  const insecure = insecureApiUrl(value);
  useEffect(() => setRevealed(false), [masked]);
  useEffect(() => {
    const validator = document.createElement("input");
    validator.type = "url";
    validator.value = value;
    input.current?.setCustomValidity(
      validator.validity.valid ? "" : "Enter a valid URL.",
    );
  }, [value]);
  return (
    <div
      className="url-field"
      onBlur={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget as Node | null))
          setRevealed(false);
      }}
    >
      <label htmlFor={id}>{label}</label>
      <div className="url-field-control">
        <input
          {...props}
          id={id}
          ref={input}
          value={value}
          type={masked && !revealed ? "password" : "url"}
          data-url-field="true"
          aria-describedby={
            [props["aria-describedby"], insecure && warningId]
              .filter(Boolean)
              .join(" ") || undefined
          }
          className={`mono${props.className ? ` ${props.className}` : ""}`}
          autoComplete="off"
          spellCheck={false}
          // Masking uses type=password; tell password managers this is no
          // credential so they neither offer to save nor fill the URL.
          data-1p-ignore=""
          data-lpignore="true"
          data-bwignore=""
          data-form-type="other"
        />
        {masked && (
          <button
            type="button"
            className="icon-button url-visibility-toggle"
            aria-label={`${revealed ? "Hide" : "Show"} ${label}`}
            aria-controls={id}
            aria-pressed={revealed}
            onClick={() => setRevealed((shown) => !shown)}
          >
            {revealed ? <EyeOffIcon size={15} /> : <EyeIcon size={15} />}
          </button>
        )}
      </div>
      {insecure && <InsecureURLWarning id={warningId} />}
    </div>
  );
}

export function InsecureURLWarning({ id }: { id?: string }) {
  return (
    <p id={id} className="notice warn">
      This http:// address is not on your local network, so the API key is sent
      unencrypted. Use https://.
    </p>
  );
}
