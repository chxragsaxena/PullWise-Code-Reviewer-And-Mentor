export interface RepoRef {
  owner: string;
  repo: string;
}

const PART = /^[A-Za-z0-9_.-]{1,100}$/;
const GITHUB_HOSTS = new Set(["github.com", "www.github.com"]);

/** Parses "owner/name" or any github.com link into a repository; null if it is neither. */
export function parseRepoRef(raw: string): RepoRef | null {
  let value = raw.trim();
  if (!value) return null;

  if (/^(https?:\/\/|github\.com\/|www\.github\.com\/)/i.test(value)) {
    try {
      const url = new URL(value.includes("://") ? value : `https://${value}`);
      if (!GITHUB_HOSTS.has(url.hostname.toLowerCase())) return null;
      value = url.pathname.replace(/^\/+/, "");
    } catch {
      return null;
    }
  }

  const [owner, rawRepo] = value.split("/");
  const repo = rawRepo?.replace(/\.git$/, "");
  if (!owner || !repo || !validPart(owner) || !validPart(repo)) return null;
  return { owner, repo };
}

/** A name part may not be only dots ("." and ".." are path steps, not names). */
const validPart = (part: string) => PART.test(part) && !/^\.+$/.test(part);

export const repoName = (ref: RepoRef) => `${ref.owner}/${ref.repo}`;
