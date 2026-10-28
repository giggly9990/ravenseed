// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { api, bytes, parseEvents, safeURL } from "./api";

describe("SSE framing", () => {
  it("retains incomplete frames and skips heartbeat comments", () => {
    const first = parseEvents(
      ': heartbeat\n\nid: 7\nevent: indexer.completed\ndata: {"status":"running"}\n\nid: 8\nevent: search.com',
    );
    expect(first.events).toEqual([
      { id: "7", name: "indexer.completed", data: '{"status":"running"}' },
    ]);
    expect(
      parseEvents(first.rest + 'pleted\ndata: {"status":"completed"}\n\n')
        .events[0].name,
    ).toBe("search.completed");
  });
  it("supports multiline data and CRLF", () => {
    expect(
      parseEvents("event: x\r\ndata: one\r\ndata: two\r\n\r\n").events[0].data,
    ).toBe("one\ntwo");
  });
});
it("does not expose script links from untrusted results", () => {
  expect(safeURL("javascript:alert(1)")).toBeUndefined();
  expect(safeURL("https://example.org/release/1")).toBe(
    "https://example.org/release/1",
  );
});
it("does not display missing size as a verified zero-byte release", () => {
  expect(bytes(0)).toBe("Unknown size");
  expect(bytes(1024 ** 3)).toBe("1.00 GiB");
  expect(bytes(700 * 1024 ** 2)).toBe("700.00 MiB");
  expect(bytes(3 * 1024 ** 4)).toBe("3.00 TiB");
  expect(bytes(512)).toBe("512 B");
});

describe("api()", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });
  const respond = (status: number, body: string) =>
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response(body, { status })),
    );

  it("returns parsed JSON and the server's error message", async () => {
    respond(200, '{"ok":true}');
    await expect(api("/x")).resolves.toEqual({ ok: true });
    respond(409, '{"error":{"code":"upgrade_changed","message":"Changed"}}');
    await expect(api("/x")).rejects.toMatchObject({
      name: "ApiError",
      status: 409,
      code: "upgrade_changed",
      message: "Changed",
    });
  });

  it("reports the status of an HTML error page instead of a SyntaxError", async () => {
    respond(502, "<html>Bad gateway</html>");
    await expect(api("/x")).rejects.toThrow("(HTTP 502)");
    respond(413, "");
    await expect(api("/x")).rejects.toThrow("too large");
  });

  it("still redirects to login when a proxy answers 401 with HTML", async () => {
    const seen = vi.fn();
    window.addEventListener("auth-required", seen);
    respond(401, "<html>Unauthorized</html>");
    await expect(api("/x")).rejects.toThrow("HTTP 401");
    window.removeEventListener("auth-required", seen);
    expect(seen).toHaveBeenCalledOnce();
  });

  it("accepts empty bodies and rejects unreadable success bodies", async () => {
    respond(200, "");
    await expect(api("/x")).resolves.toBeUndefined();
    respond(200, "<html></html>");
    await expect(api("/x")).rejects.toThrow("unreadable response");
  });
});
