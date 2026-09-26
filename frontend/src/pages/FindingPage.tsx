import { Link, useParams } from "react-router-dom";
import { useApi } from "../api/ApiProvider";
import { ApiError } from "../api/client";
import { CodeBlock } from "../components/CodeBlock";
import { Markdown } from "../components/Markdown";
import { EmptyState, ErrorState, LoadingState } from "../components/PageState";
import { SeverityBadge } from "../components/SeverityBadge";
import { SourceBadge } from "../components/SourceBadge";
import { useAsync } from "../hooks/useAsync";
import { formatLocation } from "../lib/format";
import "./FindingPage.css";

export function FindingPage() {
  const { id = "", findingId = "" } = useParams();
  const api = useApi();
  const finding = useAsync(() => api.getFinding(id, findingId), [api, id, findingId]);

  const back = (
    <Link to={`/reviews/${id}`} className="back-link">
      ← Back to review
    </Link>
  );

  if (finding.error) {
    if (finding.error instanceof ApiError && finding.error.status === 404) {
      return (
        <>
          {back}
          <EmptyState title="Finding not found">It may belong to a review that no longer exists.</EmptyState>
        </>
      );
    }
    return (
      <>
        {back}
        <ErrorState error={finding.error} onRetry={finding.reload} />
      </>
    );
  }
  if (!finding.data) return <LoadingState label="Loading finding" />;

  const f = finding.data;
  const location = formatLocation(f.file, f.line, f.endLine);

  return (
    <article className="finding">
      {back}

      <header className="finding__header">
        <div className="finding__badges">
          <SeverityBadge severity={f.severity} />
          <SourceBadge finding={f} />
        </div>
        <h1>{f.checkTitle}</h1>
        <p className="muted">
          {f.skillName} · <code>{location}</code>
        </p>
      </header>

      <section aria-labelledby="evidence-heading">
        <h2 id="evidence-heading">Evidence</h2>
        <CodeBlock caption={location}>{f.evidence}</CodeBlock>
      </section>

      <section aria-labelledby="found-heading">
        <h2 id="found-heading">What we found</h2>
        <p>{f.found}</p>
      </section>

      <section aria-labelledby="why-heading">
        <h2 id="why-heading">Why this matters</h2>
        <p>{f.why}</p>
      </section>

      <section aria-labelledby="improve-heading">
        <h2 id="improve-heading">Possible improvement</h2>
        <p>{f.improvement}</p>
      </section>

      <section aria-labelledby="learn-heading" className="finding__lesson">
        <h2 id="learn-heading">Learn</h2>
        {f.lesson ? <Markdown>{f.lesson.markdown}</Markdown> : <p className="muted">No lesson is attached to this check yet.</p>}
      </section>
    </article>
  );
}
