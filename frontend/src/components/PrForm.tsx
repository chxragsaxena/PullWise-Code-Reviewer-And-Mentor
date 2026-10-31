import { useId, useRef, useState, type FormEvent } from "react";
import { parsePrUrl } from "../lib/prUrl";
import "./PrForm.css";

const INVALID_MESSAGE = "Enter a GitHub pull request link, like https://github.com/owner/repo/pull/123";
const MAX_CHIPS = 5;

interface PrFormProps {
  /** Starts the review. Rejecting shows the error's message under the field. */
  onSubmit: (prUrl: string) => Promise<void>;
  /** Repositories ("owner/name") the user used before; offered as suggestions while typing. */
  repos?: string[];
}

const pullPrefix = (repo: string) => `https://github.com/${repo}/pull/`;

export function PrForm({ onSubmit, repos = [] }: PrFormProps) {
  const inputId = useId();
  const errorId = useId();
  const listId = useId();
  const inputRef = useRef<HTMLInputElement>(null);
  const [value, setValue] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(event: FormEvent) {
    event.preventDefault();
    const prUrl = value.trim();
    if (!parsePrUrl(prUrl)) {
      setError(INVALID_MESSAGE);
      return;
    }

    setError(null);
    setSubmitting(true);
    try {
      await onSubmit(prUrl);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not start the review.");
      setSubmitting(false);
    }
    // On success the page navigates away, so the form stays disabled.
  }

  return (
    <form className="pr-form" onSubmit={handleSubmit} noValidate>
      <label htmlFor={inputId} className="pr-form__label">
        GitHub pull request link
      </label>
      <div className="pr-form__row">
        <input
          id={inputId}
          ref={inputRef}
          className="input"
          type="text"
          inputMode="url"
          autoComplete="off"
          spellCheck={false}
          placeholder="https://github.com/owner/repo/pull/123"
          value={value}
          onChange={(e) => setValue(e.target.value)}
          aria-invalid={error !== null}
          aria-describedby={error ? errorId : undefined}
          disabled={submitting}
          list={repos.length > 0 ? listId : undefined}
        />
        {repos.length > 0 && (
          <datalist id={listId}>
            {repos.map((repo) => (
              <option key={repo} value={pullPrefix(repo)} />
            ))}
          </datalist>
        )}
        <button type="submit" className="button" disabled={submitting}>
          {submitting ? "Starting…" : "Review PR"}
        </button>
      </div>
      {repos.length > 0 && (
        <div className="pr-form__recent">
          <span>Your repositories:</span>
          {repos.slice(0, MAX_CHIPS).map((repo) => (
            <button
              key={repo}
              type="button"
              className="pr-form__chip"
              disabled={submitting}
              onClick={() => {
                setValue(pullPrefix(repo));
                inputRef.current?.focus();
              }}
            >
              {repo}
            </button>
          ))}
        </div>
      )}
      {error && (
        <p id={errorId} className="pr-form__error" role="alert">
          {error}
        </p>
      )}
    </form>
  );
}
