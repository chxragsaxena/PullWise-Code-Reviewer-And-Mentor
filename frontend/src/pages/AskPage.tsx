import { useId, useState, type FormEvent } from "react";
import { useSearchParams } from "react-router-dom";
import { ChatPanel } from "../components/ChatPanel";
import { TryExamples, type Example } from "../components/TryExamples";
import { useRecentRepos } from "../hooks/useRecentRepos";
import { parseRepoRef, repoName } from "../lib/repoRef";
import "./AskPage.css";

const EXAMPLE_REPOS: Example[] = [
  { id: "chxragsaxena/pullwise-knowledge", title: "pullwise-knowledge", detail: "Small Go service, ready in seconds" },
  { id: "go-chi/chi", title: "go-chi/chi", detail: "Go HTTP router, first answer takes about a minute" },
];

const SUGGESTIONS = [
  "What does this repository do?",
  "Where is the entry point?",
  "How is configuration handled?",
  "How are errors handled?",
];

export function AskPage() {
  const inputId = useId();
  const listId = useId();
  const myRepos = useRecentRepos();
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
            list={myRepos.length > 0 ? listId : undefined}
          />
          {myRepos.length > 0 && (
            <datalist id={listId}>
              {myRepos.map((repo) => (
                <option key={repo} value={repo} />
              ))}
            </datalist>
          )}
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
      {myRepos.length > 0 && (
        <TryExamples
          heading="Your recent repositories:"
          examples={myRepos.slice(0, 6).map((repo) => ({ id: repo, title: repo, detail: "" }))}
          onPick={(repo) => setParams({ repo })}
        />
      )}
      <TryExamples
        heading="Not sure which repository? Try one:"
        examples={EXAMPLE_REPOS}
        onPick={(repo) => setParams({ repo })}
      />
      <p className="ask__hint">Private repository? Sign in with GitHub first, and only you will be able to ask about it.</p>
    </section>
  );
}
