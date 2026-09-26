import type { ReviewEvent, ReviewStatus } from "../api/types";
import { STATUS_LABEL } from "../lib/status";
import { StatusStepper } from "./StatusStepper";
import "./ProgressView.css";

interface ProgressViewProps {
  /** Status from the latest stream event, or the last fetched status before one arrives. */
  status: ReviewStatus;
  events: readonly ReviewEvent[];
  streamError: Error | null;
}

export function ProgressView({ status, events, streamError }: ProgressViewProps) {
  const latest = events.at(-1);

  return (
    <section className="progress" aria-labelledby="progress-heading">
      <h2 id="progress-heading">Reviewing your pull request</h2>
      <p className="progress__now" role="status" aria-live="polite">
        {latest?.message ?? STATUS_LABEL[status]}
      </p>

      <StatusStepper current={status} />

      {streamError && (
        <p className="progress__error" role="alert">
          {streamError.message} Reload the page to reconnect.
        </p>
      )}

      {events.length > 0 && (
        <details className="progress__log">
          <summary>Activity log</summary>
          <ul>
            {events.map((event) => (
              <li key={event.status}>
                <time dateTime={event.at}>{new Date(event.at).toLocaleTimeString()}</time>
                <span>{event.message}</span>
              </li>
            ))}
          </ul>
        </details>
      )}
    </section>
  );
}
