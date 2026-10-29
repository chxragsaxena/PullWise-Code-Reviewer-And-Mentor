export interface PrRef {
  owner: string;
  repo: string;
  number: number;
}

const GITHUB_HOSTS = new Set(["github.com", "www.github.com"]);

/** Parses a GitHub PR link, mirroring backend/internal/github/prurl.go; scheme is optional. */
export function parsePrUrl(raw: string): PrRef | null {
  let value = raw.trim();
  if (!value) return null;
  if (!value.includes("://")) value = `https://${value}`;

  let url: URL;
  try {
    url = new URL(value);
  } catch {
    return null;
  }
  if (url.protocol !== "https:" && url.protocol !== "http:") return null;
  if (!GITHUB_HOSTS.has(url.hostname.toLowerCase())) return null;

  const [owner, repo, kind, rawNumber] = url.pathname.replace(/^\/+|\/+$/g, "").split("/");
  if (!owner || !repo || kind !== "pull" || !rawNumber || !/^\d+$/.test(rawNumber)) return null;

  const number = Number.parseInt(rawNumber, 10);
  return number >= 1 ? { owner, repo, number } : null;
}
