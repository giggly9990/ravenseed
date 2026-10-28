import { useEffect } from "react";
import { Button } from "./button";

/** Offset of the last page holding any of `total` rows. */
export function lastPageOffset(total: number, page: number) {
  return Math.max(0, Math.floor((total - 1) / page) * page);
}

/**
 * Previous/Next pager for limit/offset lists. When the total shrinks below
 * the current offset (rows dismissed, deleted or pruned), it moves back to
 * the last page that still has rows, even while hidden. Pass `total` as
 * undefined while a page is loading so the clamp waits for the real count.
 */
export function Pager({
  offset,
  total,
  page,
  onChange,
  className = "",
  buttonClassName = "sm quiet",
  showSinglePage = false,
}: {
  offset: number;
  total: number | undefined;
  page: number;
  onChange: (offset: number) => void;
  className?: string;
  buttonClassName?: string;
  /** Show the range even when everything fits on one page. */
  showSinglePage?: boolean;
}) {
  const clamped =
    total === undefined || offset === 0 || offset < total
      ? offset
      : lastPageOffset(total, page);
  useEffect(() => {
    if (clamped !== offset) onChange(clamped);
  }, [clamped, offset, onChange]);
  if (!total || (!showSinglePage && total <= page)) return null;
  return (
    <div className={`pagination ${className}`.trim()}>
      <span>
        {clamped + 1}–{Math.min(clamped + page, total)} of {total}
      </span>
      <Button
        className={buttonClassName}
        disabled={clamped === 0}
        onClick={() => onChange(Math.max(0, clamped - page))}
      >
        Previous
      </Button>
      <Button
        className={buttonClassName}
        disabled={clamped + page >= total}
        onClick={() => onChange(clamped + page)}
      >
        Next
      </Button>
    </div>
  );
}
