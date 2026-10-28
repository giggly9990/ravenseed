import { describe, expect, it } from "vitest";
import { insecureApiUrl } from "./insecure-url";

describe("insecureApiUrl", () => {
  it.each([
    "http://example.com:7476",
    "http://qui.example.org/",
    "http://203.0.113.5:8989",
    "http://8.8.8.8",
    "http://172.32.0.1",
    "http://[2001:db8::1]:5000",
    "http://[::ffff:8.8.8.8]",
    "HTTP://Example.COM",
  ])("warns for %s", (url) => {
    expect(insecureApiUrl(url)).toBe(true);
  });

  it.each([
    "https://example.com",
    "http://localhost:7476",
    "http://127.0.0.1:7476",
    "http://127.1",
    "http://[::1]:7476",
    "http://10.0.0.5",
    "http://172.16.0.1",
    "http://172.31.255.255",
    "http://192.168.1.10:8989",
    "http://169.254.1.1",
    "http://[fd12:3456::1]",
    "http://[fe80::1]",
    "http://[::ffff:192.168.1.2]",
    "http://qui:7476",
    "http://upload-assistant:5000",
    "http://nas.local",
    "http://nas.lan.",
    "http://router.home.arpa",
    "http://host.docker.internal:7476",
    "http://app.localhost",
    "",
    "not a url",
    "ftp://example.com",
  ])("does not warn for %j", (url) => {
    expect(insecureApiUrl(url)).toBe(false);
  });
});
