import { useEffect, useState } from "react";

/**
 * The current time, refreshed every `interval` ms while `active`, so
 * relative times ("3 minutes ago") keep moving without refetching data.
 */
export function useNow(interval = 30_000, active = true) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!active) return;
    setNow(Date.now());
    const timer = setInterval(() => setNow(Date.now()), interval);
    return () => clearInterval(timer);
  }, [interval, active]);
  return now;
}
