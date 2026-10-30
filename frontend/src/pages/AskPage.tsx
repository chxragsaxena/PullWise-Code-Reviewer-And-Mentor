import { useId, useState, type FormEvent } from "react";
import { useSearchParams } from "react-router-dom";
import { ChatPanel } from "../components/ChatPanel";
import { parseRepoRef, repoName } from "../lib/repoRef";
import "./AskPage.css";

const SUGGESTIONS = [
  "What does this repository do?",
  "Where is the entry point?",
  "How is configuration handled?",
  "How are errors handled?",
];

export function AskPage() {
  const inputId = useId();
  const [params, setParams] = useSearchParams();
  const selected = parseRepoRef(params.get("repo") ?? "");
  const [value, setValue] = useState("");
  const [error, setError] = useState<string | null>(null);

  function handleSubmit(event: FormEvent) {
    event.preventDefault();
    const ref = parseRepoRef(value);
    if (!ref) {
      setError("Enter a GitHub repository, like owner/name or https://github.com/owner/name");
      return;
    }
    setError(null);
    setParams({ repo: repoName(ref) });
  }

  if (selected) {
    const name = repoName(selected);
    return (
      <section className="ask">
        <div className="ask__bar">
          <h1>
            Ask about <code>{name}</code>
          </h1>
          <button type="button" className="button button--secondary" onClick={() => setParams({})}>
            Change repository
          </button>
        </div>
        <ChatPanel key={name} repo={name} subtitle={`Repository ${name}`} suggestions={SUGGESTIONS} />
      </section>
    );
  }

  return (
    <section className="ask">
      <h1>Ask about a repository</h1>
      <p className="ask__lead">
        Pick a GitHub repository and ask questions about its code. PullWise reads the repository once, then answers from
        the code itself and shows which files and lines it used.
      </p>
      <form className="ask__form" onSubmit={handleSubmit} noValidate>
        <label htmlFor={inputId} className="ask__label">
          GitHub repository
        </label>
        <div className="ask__row">
          <input
            id={inputId}
            className="input"
            value={value}
            placeholder="owner/name or https://github.com/owner/name"
            autoComplete="off"
            spellCheck={false}
            onChange={(e) => setValue(e.target.value)}
            aria-invalid={error !== null}
          />
          <button type="submit" className="button">
            Start asking
          </button>
        </div>
        {error && (
          <p className="ask__error" role="alert">
            {error}
          </p>
        )}
      </form>
      <p className="ask__hint">Private repository? Sign in with GitHub first, and only you will be able to ask about it.</p>
    </section>
  );
}
