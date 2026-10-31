import { useNavigate } from "react-router-dom";
import { useApi } from "../api/ApiProvider";
import { ErrorState, LoadingState } from "../components/PageState";
import { PrForm } from "../components/PrForm";
import { ReviewList } from "../components/ReviewList";
import { TryExamples, type Example } from "../components/TryExamples";
import { useAsync } from "../hooks/useAsync";
import { useRecentRepos } from "../hooks/useRecentRepos";
import "./HomePage.css";

/** Real, merged pull requests; the id is the link, so picking one starts a review of it. */
const EXAMPLE_PRS: Example[] = [
  {
    id: "https://github.com/gin-gonic/gin/pull/4852",
    title: "Go · gin #4852",
    detail: "Routing-tree fix",
  },
  {
    id: "https://github.com/colinhacks/zod/pull/6706",
    title: "TypeScript · zod #6706",
    detail: "Faster template literals",
  },
  {
    id: "https://github.com/pallets/flask/pull/6133",
    title: "Python · flask #6133",
    detail: "New route decorator",
  },
];

export function HomePage() {
  const api = useApi();
  const navigate = useNavigate();
  const recent = useAsync(() => api.listReviews(), [api]);
  const myRepos = useRecentRepos();

  async function startReview(prUrl: string) {
    const review = await api.createReview(prUrl);
    navigate(`/reviews/${review.id}`);
  }

  return (
    <>
      <section className="hero">
        <h1>Review a pull request. Learn why.</h1>
        <p className="hero__lead">
          Paste a GitHub pull request, or try a real one below. PullWise checks it against engineering standards and explains each problem it
          finds, with the evidence and a short lesson.
        </p>
        <PrForm onSubmit={startReview} repos={myRepos} />
        <TryExamples heading="No pull request handy? Try a real one:" examples={EXAMPLE_PRS} onPick={startReview} />
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
