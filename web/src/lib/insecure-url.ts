// Connection URLs carry an API key with every request. Plain http is fine on
// the same machine or a private network; anywhere else the key travels
// unencrypted.

const localSuffixes = [
  ".localhost",
  ".local",
  ".lan",
  ".home.arpa",
  ".internal",
];

/** Reports a plain http:// URL whose host is neither loopback nor private. */
export function insecureApiUrl(value: string): boolean {
  let url: URL;
  try {
    url = new URL(value.trim());
  } catch {
    return false;
  }
  return url.protocol === "http:" && !localHost(url.hostname);
}

function localHost(hostname: string): boolean {
  const host = hostname.toLowerCase().replace(/\.$/, "");
  if (host.startsWith("[")) return localIPv6(host.slice(1, -1));
  const ipv4 = ipv4Octets(host);
  if (ipv4) return localIPv4(ipv4);
  // Single-label names (localhost, docker service names) only resolve locally.
  return !host.includes(".") || localSuffixes.some((s) => host.endsWith(s));
}

function ipv4Octets(host: string): number[] | undefined {
  // The URL parser already normalises IPv4 forms such as 127.1.
  if (!/^\d{1,3}(\.\d{1,3}){3}$/.test(host)) return undefined;
  return host.split(".").map(Number);
}

function localIPv4([a, b]: number[]): boolean {
  return (
    a === 127 || // loopback
    a === 10 ||
    (a === 172 && b >= 16 && b <= 31) ||
    (a === 192 && b === 168) ||
    (a === 169 && b === 254) // link-local
  );
}

function localIPv6(host: string): boolean {
  if (host === "::1") return true;
  // IPv4-mapped addresses are serialised as ::ffff:7f00:1.
  const mapped = /^::ffff:([0-9a-f]{1,4}):([0-9a-f]{1,4})$/.exec(host);
  if (mapped) {
    const high = parseInt(mapped[1], 16);
    const low = parseInt(mapped[2], 16);
    return localIPv4([high >> 8, high & 255, low >> 8, low & 255]);
  }
  const first = parseInt(host.split(":")[0] || "0", 16);
  return (
    (first & 0xfe00) === 0xfc00 || // unique local fc00::/7
    (first & 0xffc0) === 0xfe80 // link-local fe80::/10
  );
}
