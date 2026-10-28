import type { KeyboardEvent } from "react";

/**
 * Arrow-key handling for a horizontal tablist with a roving tabindex: only
 * the selected tab is in the Tab order (tabIndex 0, the rest -1), and
 * Left/Right/Home/End select and focus another tab.
 */
export function onTabKeyDown<T>(
  event: KeyboardEvent<HTMLElement>,
  ids: readonly T[],
  current: T,
  select: (id: T) => void,
) {
  const last = ids.length - 1;
  const at = ids.indexOf(current);
  const next =
    event.key === "ArrowRight"
      ? at === last
        ? 0
        : at + 1
      : event.key === "ArrowLeft"
        ? at <= 0
          ? last
          : at - 1
        : event.key === "Home"
          ? 0
          : event.key === "End"
            ? last
            : -1;
  if (next < 0) return;
  event.preventDefault();
  select(ids[next]);
  event.currentTarget
    .closest('[role="tablist"]')
    ?.querySelectorAll<HTMLElement>('[role="tab"]')
    [next]?.focus();
}
