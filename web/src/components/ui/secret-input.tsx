import { useState, type InputHTMLAttributes } from "react";
import { EyeIcon, EyeOffIcon } from "../icons";

/**
 * Password-style input with a Show/Hide button. The raven peek perches on
 * it while focused and covers its eyes while the value is shown.
 */
export function SecretInput({
  className,
  ...props
}: Omit<InputHTMLAttributes<HTMLInputElement>, "type">) {
  const [shown, setShown] = useState(false);
  return (
    <span className={`secret-input${className ? ` ${className}` : ""}`}>
      <input
        {...props}
        type={shown ? "text" : "password"}
        data-raven-peek=""
        autoComplete={props.autoComplete ?? "new-password"}
        spellCheck={false}
      />
      <button
        type="button"
        className="icon-button"
        aria-label={shown ? "Hide value" : "Show value"}
        aria-pressed={shown}
        disabled={props.disabled}
        onClick={() => setShown((value) => !value)}
      >
        {shown ? <EyeOffIcon size={15} /> : <EyeIcon size={15} />}
      </button>
    </span>
  );
}
