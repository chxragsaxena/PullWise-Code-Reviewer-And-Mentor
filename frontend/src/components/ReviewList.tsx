import { Link } from "react-router-dom";
import type { ReviewSummary } from "../api/types";
import { formatRelativeTime } from "../lib/format";
import { STATUS_LABEL } from "../lib/status";
import { SEVERITIES } from "../api/types";
import { SEVERITY_LABEL } from "../lib/severity";
import { EmptyState } from "./PageState";
import "./ReviewList.css";

function Outcome({ review }: { review: ReviewSummary }) {
  if (review.status === "failed") return <span className="status-pill status-pill--failed">Failed</span>;
  if (review.status !== "completed") {
    return <span className="status-pill status-pill--running">{STATUS_LABEL[review.status]}</span>;
  }

  const shown = SEVERITIES.filter((s) => review.counts[s] > 0);
  if (shown.length === 0) return <span className="status-pill status-pill--clean">No issues</span>;
  return (
    <span className="count-pills">
      {shown.map((s) => (
        <span key={s} className={`badge badge--${s}`} title={SEVERITY_LABEL[s]}>
          {review.counts[s]} {SEVERITY_LABEL[s]}
        </span>
      ))}
    </span>
  );
}

export function ReviewList({ reviews }: { reviews: readonly ReviewSummary[] }) {
  if (reviews.length === 0) {
    return <EmptyState title="No reviews yet">Paste a pull request link above to start your first review.</EmptyState>;
  }

  return (
    <ul className="review-list">
      {reviews.map((review) => {
        const pr = review.pullRequest;
        return (
          <li key={review.id}>
            <Link to={`/reviews/${review.id}`} className="review-card">
              <span className="review-card__main">
                <span className="review-card__repo">
                  {pr.owner}/{pr.repo} <span className="muted">#{pr.number}</span>
                </span>
                <span className="review-card__title">{pr.title}</span>
              </span>
              <span className="review-card__meta">
                <Outcome review={review} />
                <time className="muted" dateTime={review.createdAt}>
                  {formatRelativeTime(review.createdAt)}
                </time>
              </span>
            </Link>
          </li>
        );
      })}
    </ul>
  );
}
