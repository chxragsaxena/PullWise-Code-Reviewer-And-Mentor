import type { PullRequest } from "../api/types";
import "./PrSummary.css";

export function PrSummary({ pullRequest: pr }: { pullRequest: PullRequest }) {
  return (
    <header className="pr-summary">
      <p className="pr-summary__repo">
        <a href={pr.url || `https://github.com/${pr.owner}/${pr.repo}/pull/${pr.number}`} target="_blank" rel="noopener noreferrer">
          {pr.owner}/{pr.repo} #{pr.number}
        </a>
      </p>
      {/* The title is empty until the backend has fetched the pull request. */}
      <h1 className="pr-summary__title">{pr.title || `Pull request #${pr.number}`}</h1>
      <p className="pr-summary__meta">
        {pr.author && <span>by {pr.author}</span>}
        {pr.filesChanged > 0 && (
          <span>
            {pr.filesChanged} {pr.filesChanged === 1 ? "file" : "files"} changed
          </span>
        )}
        {pr.filesChanged > 0 && (
          <span className="pr-summary__diff">
            <span className="pr-summary__add">+{pr.additions}</span>{" "}
            <span className="pr-summary__del">−{pr.deletions}</span>
          </span>
        )}
        {pr.headSha && (
          <span>
            commit <code>{pr.headSha.slice(0, 7)}</code>
          </span>
        )}
      </p>
    </header>
  );
}
