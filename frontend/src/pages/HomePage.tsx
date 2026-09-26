import { useNavigate } from "react-router-dom";
import { useApi } from "../api/ApiProvider";
import { ErrorState, LoadingState } from "../components/PageState";
import { PrForm } from "../components/PrForm";
import { ReviewList } from "../components/ReviewList";
import { useAsync } from "../hooks/useAsync";
import "./HomePage.css";

export function HomePage() {
  const api = useApi();
  const navigate = useNavigate();
  const recent = useAsync(() => api.listReviews(), [api]);

  async function startReview(prUrl: string) {
    const review = await api.createReview(prUrl);
    navigate(`/reviews/${review.id}`);
  }

  return (
    <>
      <section className="hero">
        <h1>Review a pull request. Learn why.</h1>
        <p className="hero__lead">
          Paste a GitHub pull request. PullWise checks it against engineering standards and explains each problem it
          finds, with the evidence and a short lesson.
        </p>
        <PrForm onSubmit={startReview} />
        {api.kind === "mock" && (
          <p className="hero__hint">
            Demo mode: reviews are simulated in your browser. Any valid pull request link works, and pull request #404
            simulates a failure.
          </p>
        )}
      </section>

      <section aria-labelledby="recent-heading" className="recent">
        <h2 id="recent-heading">Recent reviews</h2>
        {recent.error ? (
          <ErrorState error={recent.error} onRetry={recent.reload} />
        ) : recent.data ? (
          <ReviewList reviews={recent.data} />
        ) : (
          <LoadingState label="Loading recent reviews" />
        )}
      </section>
    </>
  );
}
