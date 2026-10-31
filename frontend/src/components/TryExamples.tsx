import { useState } from "react";
import "./TryExamples.css";

export interface Example {
  id: string;
  title: string;
  detail: string;
}

interface TryExamplesProps {
  heading: string;
  examples: Example[];
  /** Runs when an example is picked. Rejecting shows the error's message under the examples. */
  onPick: (id: string) => Promise<void> | void;
}

/** One-click starting points, so nobody has to find a link before trying the app. */
export function TryExamples({ heading, examples, onPick }: TryExamplesProps) {
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  async function pick(id: string) {
    setError(null);
    setBusy(id);
    try {
      await onPick(id);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not start that example.");
      setBusy(null);
    }
    // On success the page usually navigates away, so the buttons stay disabled.
  }

  return (
    <div className="try">
      <p className="try__heading">{heading}</p>
      <ul className="try__list">
        {examples.map((example) => (
          <li key={example.id}>
            <button type="button" className="try__item" disabled={busy !== null} onClick={() => pick(example.id)}>
              <span className="try__title">{busy === example.id ? "Starting…" : example.title}</span>
              {example.detail && <span className="try__detail">{example.detail}</span>}
            </button>
          </li>
        ))}
      </ul>
      {error && (
        <p className="try__error" role="alert">
          {error}
        </p>
      )}
    </div>
  );
}
