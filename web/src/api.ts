export async function api<T>(path: string, options?: RequestInit): Promise<T> {
  const response = await fetch(`/api/v1${path}`, {
    ...options,
    credentials: "same-origin",
    headers: {
      ...(options?.body ? { "Content-Type": "application/json" } : {}),
      ...options?.headers,
    },
  });
  // Parse after reading the status: a proxy's HTML 401/502 page must still
  // report the status (and send 401s to login), not a raw SyntaxError.
  const text = await response.text();
  let body: { error?: { code?: string; message?: string } } | undefined;
  let parsed = true;
  try {
    body = text ? JSON.parse(text) : undefined;
  } catch {
    parsed = false;
  }
  if (!response.ok) {
    if (response.status === 401)
      window.dispatchEvent(new Event("auth-required"));
    throw new ApiError(
      response.status,
      body?.error?.code ?? "",
      body?.error?.message ?? fallbackMessage(response.status),
    );
  }
  if (!parsed) throw new Error("The server returned an unreadable response");
  return body as T;
}

/** A failed API call; `code` is the server's error code when it sent one. */
export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

/** The server error code and HTTP status of a failed call (duck-typed, so
 * test doubles of this module need not export ApiError). */
export function apiFailure(error: unknown): { status: number; code: string } {
  const e = error as Partial<ApiError> | null;
  return {
    status: typeof e?.status === "number" ? e.status : 0,
    code: typeof e?.code === "string" ? e.code : "",
  };
}

/** Messages for responses without a JSON error body (proxies, timeouts). */
function fallbackMessage(status: number) {
  if (status === 413) return "The request was too large.";
  if (status === 429) return "Too many requests. Try again later.";
  if (status === 502)
    return "An upstream service did not respond correctly (HTTP 502).";
  if (status === 504) return "The request timed out (HTTP 504).";
  return `Request failed (HTTP ${status})`;
}
export interface StreamEvent {
  name: string;
  data: string;
  id?: string;
}
export function parseEvents(buffer: string): {
  events: StreamEvent[];
  rest: string;
} {
  const normalized = buffer.replace(/\r\n/g, "\n");
  const blocks = normalized.split("\n\n");
  const rest = blocks.pop() ?? "";
  const events: StreamEvent[] = [];
  for (const block of blocks) {
    let name = "message",
      id: string | undefined;
    const data: string[] = [];
    for (const line of block.split("\n")) {
      if (line.startsWith("event:")) name = line.slice(6).trim();
      if (line.startsWith("id:")) id = line.slice(3).trim();
      if (line.startsWith("data:")) data.push(line.slice(5).trimStart());
    }
    if (data.length) events.push({ name, data: data.join("\n"), id });
  }
  return { events, rest };
}
export function bytes(n: number) {
  if (n <= 0) return "Unknown size";
  const units = ["B", "KiB", "MiB", "GiB", "TiB", "PiB"];
  let unit = 0;
  while (unit < units.length - 1 && n >= 1024 ** (unit + 1)) unit++;
  return unit === 0
    ? `${n} B`
    : `${(n / 1024 ** unit).toFixed(2)} ${units[unit]}`;
}
export function safeURL(value: string) {
  try {
    const u = new URL(value);
    return ["http:", "https:"].includes(u.protocol) ? value : undefined;
  } catch {
    return undefined;
  }
}
