import { useEffect } from "react";
import { Link, useParams } from "react-router-dom";
import { useApi } from "../api/ApiProvider";
import { ApiError } from "../api/client";
import { EmptyState, ErrorState, LoadingState } from "../components/PageState";
import { PrSummary } from "../components/PrSummary";
import { ProgressView } from "../components/ProgressView";
import { ReviewWorkspace } from "../components/ReviewWorkspace";
import { useAsync } from "../hooks/useAsync";
import { useReviewProgress } from "../hooks/useReviewProgress";
import { isTerminal } from "../lib/status";

export function ReviewPage() {
  const { id = "" } = useParams();
  const api = useApi();

  const review = useAsync(() => api.getReview(id), [api, id]);
  const fetchedStatus = review.data?.status;
  const inProgress = fetchedStatus !== undefined && !isTerminal(fetchedStatus);

  const progress = useReviewProgress(id, inProgress);
  const streamedStatus = progress.latest?.status;
  const reload = review.reload;

  // When the stream reports the end, fetch the finished review (with findings).
  useEffect(() => {
    if (streamedStatus && isTerminal(streamedStatus)) reload();
  }, [streamedStatus, reload]);

  if (review.error) {
    if (review.error instanceof ApiError && review.error.code === "review_not_found") {
      return (
        <EmptyState title="Review not found">
          It may have been removed. <Link to="/">Start a new review</Link>.
        </EmptyState>
      );
    }
    return <ErrorState error={review.error} onRetry={reload} />;
  }
  if (!review.data) return <LoadingState label="Loading review" />;

  const data = review.data;

  if (data.status === "failed") {
    return (
      <>
        <PrSummary pullRequest={data.pullRequest} />
        <div className="page-state page-state--error" role="alert">
          <strong>The review could not be completed</strong>
          <span>{data.error?.message ?? "An unknown error occurred."}</span>
          <Link to="/" className="button button--secondary">
            Try another pull request
          </Link>
        </div>
      </>
    );
  }

  if (data.status === "completed") return <ReviewWorkspace review={data} />;

  return (
    <>
      <PrSummary pullRequest={data.pullRequest} />
      <ProgressView status={streamedStatus ?? data.status} events={progress.events} streamError={progress.error} />
    </>
  );
}
