import type { PullRequest } from "../api/types";
import "./PrSummary.css";

export function PrSummary({ pullRequest: pr }: { pullRequest: PullRequest }) {
  return (
    <header className="pr-summary">
      <p className="pr-summary__repo">
        <a href={pr.url} target="_blank" rel="noopener noreferrer">
          {pr.owner}/{pr.repo} #{pr.number}
        </a>
      </p>
      <h1 className="pr-summary__title">{pr.title}</h1>
      <p className="pr-summary__meta">
        <span>by {pr.author}</span>
        <span>
          {pr.filesChanged} {pr.filesChanged === 1 ? "file" : "files"} changed
        </span>
        <span className="pr-summary__diff">
          <span className="pr-summary__add">+{pr.additions}</span>{" "}
          <span className="pr-summary__del">−{pr.deletions}</span>
        </span>
        <span>
          commit <code>{pr.headSha.slice(0, 7)}</code>
        </span>
      </p>
    </header>
  );
}
