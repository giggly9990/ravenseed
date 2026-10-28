import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
} from "react";
import { RavenMarkIcon } from "./icons";

/* ------------------------------------------------------------------ */
/* Static reaction (empty states)                                       */
/* ------------------------------------------------------------------ */

export function RavenReaction({
  message,
  crossedEye = false,
  size = 64,
}: {
  message: string;
  crossedEye?: boolean;
  size?: number;
}) {
  return (
    <div
      className={`raven-reaction${crossedEye ? " raven-reaction-error" : ""}`}
      aria-hidden="true"
    >
      <span className="raven-bubble">{message}</span>
      <RavenMarkIcon size={size} crossedEye={crossedEye} color="currentColor" />
    </div>
  );
}

/* ------------------------------------------------------------------ */
/* xD: errors. The same message coming back escalates xD → xDD → xDDD.  */
/* ------------------------------------------------------------------ */

const REPEAT_WINDOW_MS = 120_000;
const recentErrors = new Map<string, { level: number; endedAt: number }>();

function nextLevel(key: string | undefined) {
  if (!key) return 1;
  const previous = recentErrors.get(key);
  if (!previous) return 1;
  // endedAt 0: the previous notice is still mounted and being replaced by a
  // fresh error with the same message, which counts as a repeat too.
  if (previous.endedAt && Date.now() - previous.endedAt > REPEAT_WINDOW_MS)
    return 1;
  return Math.min(previous.level + 1, 3);
}

export function RavenError({
  children,
  action,
}: {
  children: ReactNode;
  action?: ReactNode;
}) {
  const key = typeof children === "string" ? children : undefined;
  // Read once per mounted instance; StrictMode's simulated remount keeps it.
  const [level] = useState(() => nextLevel(key));
  useEffect(() => {
    if (!key) return;
    recentErrors.set(key, { level, endedAt: 0 });
    return () => {
      // A newer notice for the same message may already own the entry.
      const current = recentErrors.get(key);
      if (!current || current.level <= level)
        recentErrors.set(key, { level, endedAt: Date.now() });
    };
  }, [key, level]);
  return (
    <div role="alert" className="notice error raven-error">
      <span className="raven-error-mark" aria-hidden="true">
        <RavenMarkIcon size={40} crossedEye color="currentColor" />
        <span className="raven-xd">x{"D".repeat(level)}</span>
      </span>
      <div className="raven-error-body">{children}</div>
      {action && <div className="raven-error-action">{action}</div>}
    </div>
  );
}

/** Test helper: forget remembered errors. */
export function resetRavenErrors() {
  recentErrors.clear();
}

/* ------------------------------------------------------------------ */
/* 👀 Peek: perches on the focused secret field and watches the caret.  */
/* ------------------------------------------------------------------ */

function isSecretField(node: EventTarget | null): node is HTMLInputElement {
  if (!(node instanceof HTMLInputElement)) return false;
  if (node.dataset.urlField !== undefined) return false;
  return node.type === "password" || node.dataset.ravenPeek !== undefined;
}

function sameSecretGroup(next: Element | null, anchor: HTMLInputElement) {
  const group = anchor.closest(".secret-input");
  return !!group && !!next && group.contains(next);
}

type PeekState = {
  anchor: HTMLInputElement;
  rect: DOMRect;
  look: number;
  covered: boolean;
};

export function RavenPeek() {
  const [peek, setPeek] = useState<PeekState | null>(null);
  const anchorRef = useRef<HTMLInputElement | null>(null);

  const measure = useCallback((anchor: HTMLInputElement) => {
    const rect = anchor.getBoundingClientRect();
    const caret = anchor.selectionStart ?? anchor.value.length;
    const width = Math.max(anchor.clientWidth, 1);
    const look = Math.max(0, Math.min(1, (caret * 7.5) / width));
    setPeek({
      anchor,
      rect,
      look,
      covered: anchor.type === "text",
    });
  }, []);

  useEffect(() => {
    let observer: MutationObserver | undefined;
    const attach = (anchor: HTMLInputElement) => {
      anchorRef.current = anchor;
      observer?.disconnect();
      observer = new MutationObserver(() => measure(anchor));
      observer.observe(anchor, { attributes: true, attributeFilter: ["type"] });
      measure(anchor);
    };
    const detach = () => {
      anchorRef.current = null;
      observer?.disconnect();
      setPeek(null);
    };
    const onFocusIn = (event: FocusEvent) => {
      if (isSecretField(event.target)) attach(event.target);
    };
    const onFocusOut = (event: FocusEvent) => {
      const anchor = anchorRef.current;
      if (!anchor || event.target !== anchor) return;
      const next = event.relatedTarget as Element | null;
      if (isSecretField(next)) return; // focusin attaches it
      if (sameSecretGroup(next, anchor)) return; // the show/hide button
      detach();
    };
    const onGroupBlur = (event: FocusEvent) => {
      const anchor = anchorRef.current;
      if (!anchor || event.target === anchor) return;
      if (!sameSecretGroup(event.target as Element, anchor)) return;
      const next = event.relatedTarget as Element | null;
      if (next !== anchor && !isSecretField(next)) detach();
    };
    const onActivity = (event: Event) => {
      const anchor = anchorRef.current;
      if (anchor && event.target === anchor) measure(anchor);
    };
    const onViewport = () => {
      const anchor = anchorRef.current;
      if (anchor) measure(anchor);
    };
    document.addEventListener("focusin", onFocusIn);
    document.addEventListener("focusout", onFocusOut);
    document.addEventListener("focusout", onGroupBlur);
    document.addEventListener("input", onActivity, true);
    document.addEventListener("keyup", onActivity, true);
    document.addEventListener("click", onActivity, true);
    window.addEventListener("scroll", onViewport, true);
    window.addEventListener("resize", onViewport);
    if (isSecretField(document.activeElement)) attach(document.activeElement);
    return () => {
      observer?.disconnect();
      document.removeEventListener("focusin", onFocusIn);
      document.removeEventListener("focusout", onFocusOut);
      document.removeEventListener("focusout", onGroupBlur);
      document.removeEventListener("input", onActivity, true);
      document.removeEventListener("keyup", onActivity, true);
      document.removeEventListener("click", onActivity, true);
      window.removeEventListener("scroll", onViewport, true);
      window.removeEventListener("resize", onViewport);
    };
  }, [measure]);

  if (!peek) return null;
  const style: CSSProperties = {
    left: peek.rect.right + 8,
    top: peek.rect.top + 6,
  };
  const shift = peek.look * 2.4;
  return (
    <div
      className={`raven-float raven-peek${peek.covered ? " covered" : ""}`}
      style={style}
      data-testid="raven-peek"
      aria-hidden="true"
    >
      <span className="raven-peek-eyes">
        {peek.covered ? (
          <svg width="24" height="13" viewBox="0 0 22 12">
            <path
              d="M1.5 7 Q5.5 3.5 9.5 7 M12.5 7 Q16.5 3.5 20.5 7"
              fill="none"
              stroke="var(--bubble-ink)"
              strokeWidth="1.6"
              strokeLinecap="round"
            />
          </svg>
        ) : (
          <svg width="24" height="13" viewBox="0 0 22 12">
            <circle
              cx="5.5"
              cy="6"
              r="5"
              fill="#fff"
              stroke="var(--bubble-ink)"
              strokeWidth="1.2"
            />
            <circle
              cx="16.5"
              cy="6"
              r="5"
              fill="#fff"
              stroke="var(--bubble-ink)"
              strokeWidth="1.2"
            />
            <g
              className="pupil"
              style={{ transform: `translateX(${shift}px)` }}
              fill="var(--bubble-ink)"
            >
              <circle cx="3.4" cy="7.4" r="2.2" />
              <circle cx="14.4" cy="7.4" r="2.2" />
            </g>
          </svg>
        )}
      </span>
      <RavenMarkIcon size={46} color="var(--text)" />
    </div>
  );
}

/* ------------------------------------------------------------------ */
/* got coke?: hops onto whatever shows the new candidates.              */
/* ------------------------------------------------------------------ */

const COKE_MS = 6000;

export type CokeOptions = {
  /** Element the raven should sit next to, e.g. the Opportunities tab. */
  anchor?: HTMLElement | null;
  count: number;
  /** Target indexer name, e.g. "Orbit". */
  target?: string;
  /** Called by the bubble's "show them" button. */
  onShow?: () => void;
};

type Cue = CokeOptions & { id: number };

function cokeText(cue: Cue) {
  const noun = cue.count === 1 ? "opportunity" : "opportunities";
  return `${cue.count} ${noun}${cue.target ? ` on ${cue.target}` : ""}`;
}

function CokeRaven({
  cue,
  dismiss,
  hold,
}: {
  cue: Cue;
  dismiss: () => void;
  /** Pauses the auto-dismiss while the pointer or focus is on the bubble. */
  hold: (held: boolean) => void;
}) {
  const [rect, setRect] = useState<DOMRect | null>(
    () => cue.anchor?.getBoundingClientRect() ?? null,
  );
  useLayoutEffect(() => {
    const update = () => setRect(cue.anchor?.getBoundingClientRect() ?? null);
    update();
    window.addEventListener("scroll", update, true);
    window.addEventListener("resize", update);
    return () => {
      window.removeEventListener("scroll", update, true);
      window.removeEventListener("resize", update);
    };
  }, [cue.anchor]);
  const style: CSSProperties = rect
    ? {
        left: Math.max(8, rect.left + rect.width / 2 - 28),
        top: rect.bottom + 4,
      }
    : { right: 24, bottom: 24 };
  return (
    <div className="raven-float raven-coke" style={style}>
      <RavenMarkIcon size={56} color="var(--text)" aria-hidden="true" />
      <div
        className="raven-bubble left"
        onMouseEnter={() => hold(true)}
        onMouseLeave={() => hold(false)}
        onFocus={() => hold(true)}
        onBlur={(e) => {
          if (!e.currentTarget.contains(e.relatedTarget as Node | null))
            hold(false);
        }}
      >
        <span>got coke?</span>
        <small>
          {cokeText(cue)}
          {cue.onShow && (
            <>
              {" · "}
              <button
                type="button"
                className="link-button"
                onClick={() => {
                  cue.onShow?.();
                  dismiss();
                }}
              >
                show them
              </button>
            </>
          )}
        </small>
      </div>
    </div>
  );
}

export function useRavenReaction() {
  const [cue, setCue] = useState<Cue | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const sequence = useRef(0);
  const hide = useCallback(() => {
    clearTimeout(timer.current);
    setCue(null);
  }, []);
  const show = useCallback(
    (
      // One reaction today; the name keeps call sites readable.
      _kind: "opportunities",
      options: CokeOptions,
    ) => {
      clearTimeout(timer.current);
      setCue({ ...options, id: ++sequence.current });
      timer.current = setTimeout(() => setCue(null), COKE_MS);
    },
    [],
  );
  const hold = useCallback((held: boolean) => {
    clearTimeout(timer.current);
    if (!held) timer.current = setTimeout(() => setCue(null), COKE_MS);
  }, []);
  useEffect(() => () => clearTimeout(timer.current), []);
  return {
    show,
    hide,
    reaction: (
      <>
        {/* Always mounted, so screen readers announce text put into it. */}
        <span className="visually-hidden" role="status">
          {cue ? `got coke? ${cokeText(cue)}` : ""}
        </span>
        {cue && <CokeRaven key={cue.id} cue={cue} dismiss={hide} hold={hold} />}
      </>
    ),
  };
}

/* ------------------------------------------------------------------ */
/* spread the seed: the raven flies along the search progress line and */
/* drops one seed per indexer as it answers.                            */
/* ------------------------------------------------------------------ */

export type SeedState = "pending" | "done" | "failed";

export function SeedProgress({
  items,
  summary,
}: {
  items: { key: string | number; label: string; state: SeedState }[];
  /** Shown on the right of the caption, e.g. "4 of 7 indexers · 23 results". */
  summary?: ReactNode;
}) {
  const total = Math.max(items.length, 1);
  const finished = items.filter((i) => i.state !== "pending").length;
  const complete = items.length > 0 && finished === items.length;
  const fraction = finished / total;
  return (
    <div className={`seed-progress${complete ? " complete" : ""}`}>
      <div
        className="seed-track"
        role="progressbar"
        aria-label="Indexers answered"
        aria-valuemin={0}
        aria-valuemax={items.length}
        aria-valuenow={finished}
      >
        <span className="seed-line">
          <span style={{ width: `${fraction * 100}%` }} />
        </span>
        {items.map((item, index) => (
          <span
            key={item.key}
            title={`${item.label}: ${item.state === "pending" ? "searching" : item.state === "done" ? "answered" : "failed"}`}
            className={`seed${item.state === "pending" ? "" : ` ${item.state}`}`}
            style={{ left: `${((index + 0.5) / total) * 100}%` }}
          />
        ))}
        <span
          className="seed-raven"
          style={{ left: `${Math.min(fraction, 0.97) * 100}%` }}
          aria-hidden="true"
        >
          <RavenMarkIcon size={28} color="currentColor" />
        </span>
      </div>
      <div className="seed-caption">
        {complete ? <span /> : <em>spread the seed</em>}
        {summary && <span>{summary}</span>}
      </div>
    </div>
  );
}
