import { useEffect, useState } from "react";

/**
 * `value`, once it has stopped changing for `delay` ms. Compared by JSON so
 * an object rebuilt on every render (such as a filter set) only restarts the
 * timer when its contents change.
 */
export function useDebouncedValue<T>(value: T, delay = 300): T {
  const [settled, setSettled] = useState(value);
  const key = JSON.stringify(value);
  useEffect(() => {
    if (key === JSON.stringify(settled)) return;
    const timer = setTimeout(() => setSettled(value), delay);
    return () => clearTimeout(timer);
    // `value` is represented by `key`; `settled` only matters through `key`.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key, delay]);
  return settled;
}
