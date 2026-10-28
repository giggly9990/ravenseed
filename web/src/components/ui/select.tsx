import {
  Children,
  Fragment,
  isValidElement,
  useId,
  useRef,
  useState,
  type ReactNode,
} from "react";
import * as Menu from "@radix-ui/react-dropdown-menu";
import { CheckIcon, ChevronDownIcon } from "../icons";

type Option = { value: string; label: ReactNode; disabled?: boolean };
type Change = {
  target: { value: string; selectedOptions: { value: string }[] };
};

function optionsFrom(children: ReactNode): Option[] {
  return Children.toArray(children).flatMap((child) => {
    if (
      !isValidElement<{
        value?: string | number;
        children?: ReactNode;
        disabled?: boolean;
      }>(child)
    )
      return [];
    if (child.type === Fragment) return optionsFrom(child.props.children);
    return [
      {
        value: String(child.props.value ?? ""),
        label: child.props.children,
        disabled: child.props.disabled,
      },
    ];
  });
}

export function Select({
  value,
  onChange,
  children,
  multiple,
  disabled,
  "aria-label": name,
  triggerLabel,
  required,
}: {
  value: string | number | readonly string[];
  onChange: (event: Change) => void;
  children: ReactNode;
  multiple?: boolean;
  disabled?: boolean;
  required?: boolean;
  "aria-label": string;
  triggerLabel?: ReactNode;
}) {
  const trigger = useRef<HTMLButtonElement>(null);
  const valueId = useId();
  /** Set by a pointer press on the trigger, which Radix already handles. */
  const pressed = useRef(false);
  const [open, setOpen] = useState(false);
  const options = optionsFrom(children);
  const values = (Array.isArray(value) ? value : [value]).map(String);
  const selected = options.filter(
    (o) => values.includes(o.value) && (!multiple || o.value !== ""),
  );
  const display =
    triggerLabel ??
    (selected.length
      ? selected
          .map((o) => o.label)
          .reduce<ReactNode[]>(
            (all, label, i) => [...all, ...(i ? [", "] : []), label],
            [],
          )
      : (options.find((o) => o.value === "")?.label ?? "Choose..."));
  const change = (next: string[]) =>
    onChange({
      target: {
        value: next[0] ?? "",
        selectedOptions: next.map((v) => ({ value: v })),
      },
    });
  return (
    <Menu.Root
      modal={false}
      open={open}
      // A disabled <fieldset> disables the trigger without Radix knowing.
      onOpenChange={(next) =>
        setOpen(next && !trigger.current?.matches(":disabled"))
      }
    >
      {required && (
        <select
          className="select-validation"
          aria-hidden="true"
          tabIndex={-1}
          required
          disabled={disabled}
          value={values[0] ?? ""}
          onChange={(e) => change([e.target.value])}
          onInvalid={(e) => {
            e.preventDefault();
            trigger.current?.focus();
            setOpen(true);
          }}
        >
          <option value="" />
          {options
            .filter((o) => o.value !== "")
            .map((o) => (
              <option key={o.value} value={o.value} disabled={o.disabled}>
                {o.value}
              </option>
            ))}
        </select>
      )}
      <Menu.Trigger asChild>
        <button
          type="button"
          ref={trigger}
          className="select-trigger"
          aria-label={name}
          // aria-label replaces the button text; describe it with the value.
          aria-describedby={valueId}
          disabled={disabled}
          onPointerDown={() => {
            pressed.current = true;
          }}
          // A click without a press on the trigger came from its <label>
          // (Radix opens on pointerdown/keydown only): open the menu too.
          onClick={() => {
            if (!pressed.current && !open) setOpen(true);
            pressed.current = false;
          }}
        >
          <span id={valueId}>{display}</span>
          <ChevronDownIcon size={14} aria-hidden="true" />
        </button>
      </Menu.Trigger>
      <Menu.Portal>
        <Menu.Content
          className="select-menu"
          sideOffset={6}
          collisionPadding={12}
          align="start"
          aria-label={name}
        >
          {multiple && (
            <>
              <Menu.Item
                className="select-option select-action"
                onSelect={(e) => {
                  e.preventDefault();
                  change(
                    options
                      .filter((o) => !o.disabled && o.value !== "")
                      .map((o) => o.value),
                  );
                }}
              >
                Select all
              </Menu.Item>
              <Menu.Item
                className="select-option select-action"
                onSelect={(e) => {
                  e.preventDefault();
                  change([]);
                }}
              >
                Clear
              </Menu.Item>
              <Menu.Separator className="select-separator" />
              {options
                .filter((o) => o.value !== "")
                .map((o) => (
                  <Menu.CheckboxItem
                    key={o.value}
                    className="select-option"
                    checked={values.includes(o.value)}
                    disabled={o.disabled}
                    onSelect={(e) => e.preventDefault()}
                    onCheckedChange={(checked) =>
                      change(
                        checked
                          ? [...values.filter(Boolean), o.value]
                          : values.filter((v) => v !== o.value),
                      )
                    }
                  >
                    <span className="select-check">
                      <Menu.ItemIndicator>
                        <CheckIcon size={13} />
                      </Menu.ItemIndicator>
                    </span>
                    {o.label}
                  </Menu.CheckboxItem>
                ))}
            </>
          )}
          {!multiple && (
            <Menu.RadioGroup
              value={values[0] ?? ""}
              onValueChange={(v) => change([v])}
            >
              {options.map((o) => (
                <Menu.RadioItem
                  key={o.value}
                  className="select-option"
                  value={o.value}
                  disabled={o.disabled}
                >
                  <span className="select-check">
                    <Menu.ItemIndicator>
                      <CheckIcon size={13} />
                    </Menu.ItemIndicator>
                  </span>
                  {o.label}
                </Menu.RadioItem>
              ))}
            </Menu.RadioGroup>
          )}
          {options.length === 0 && (
            <Menu.Label className="select-empty">
              No options available
            </Menu.Label>
          )}
        </Menu.Content>
      </Menu.Portal>
    </Menu.Root>
  );
}
