import { useEffect, useId, useRef, useState, type FormEvent, type KeyboardEvent } from "react";
import { useApi } from "../api/ApiProvider";
import { ASK_LIMITS, type AskSource, type ChatTurn } from "../api/types";
import { Markdown } from "./Markdown";
import "./ChatPanel.css";

interface ChatPanelProps {
  /** "owner/name" when asking about a repository on its own. */
  repo?: string;
  /** Ask about this review's repository and findings instead. */
  reviewId?: string;
  /** What the chat is about, shown under the title. */
  subtitle: string;
  suggestions: string[];
}

interface Message {
  role: "user" | "assistant";
  content: string;
  sources?: AskSource[];
}

export function ChatPanel({ repo, reviewId, subtitle, suggestions }: ChatPanelProps) {
  const api = useApi();
  const inputId = useId();
  const [messages, setMessages] = useState<Message[]>([]);
  const [draft, setDraft] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const endRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    endRef.current?.scrollIntoView?.({ block: "end" });
  }, [messages, busy]);

  async function send(raw: string) {
    const question = raw.trim();
    if (!question || busy) return;

    const history: ChatTurn[] = messages.slice(-ASK_LIMITS.maxHistory).map((m) => ({ role: m.role, content: m.content }));
    setError(null);
    setBusy(true);
    setMessages((prev) => [...prev, { role: "user", content: question }]);
    setDraft("");
    try {
      const reply = await api.ask({ repo, reviewId, question, history });
      setMessages((prev) => [...prev, { role: "assistant", content: reply.answer, sources: reply.sources }]);
    } catch (err) {
      setMessages((prev) => prev.slice(0, -1));
      setDraft(question);
      setError(err instanceof Error ? err.message : "Could not answer that right now.");
    } finally {
      setBusy(false);
    }
  }

  function handleSubmit(event: FormEvent) {
    event.preventDefault();
    void send(draft);
  }

  function handleKeyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      void send(draft);
    }
  }

  return (
    <section className="chat" aria-label="Ask about the code">
      <header className="chat__header">
        <h2>Ask about the code</h2>
        <p>{subtitle}</p>
      </header>

      <div className="chat__log" role="log" aria-live="polite">
        {messages.length === 0 && (
          <div className="chat__empty">
            <p>Ask anything about this code. Answers cite the files and lines they came from.</p>
            <div className="chat__suggestions">
              {suggestions.map((s) => (
                <button key={s} type="button" className="chat__chip" onClick={() => void send(s)} disabled={busy}>
                  {s}
                </button>
              ))}
            </div>
          </div>
        )}

        {messages.map((m, i) => (
          <div key={i} className={`chat__message chat__message--${m.role}`}>
            {m.role === "user" ? <p>{m.content}</p> : <Markdown>{m.content}</Markdown>}
            {m.sources && m.sources.length > 0 && <Sources sources={m.sources} />}
          </div>
        ))}

        {busy && (
          <div className="chat__message chat__message--assistant chat__thinking" role="status">
            <span className="spinner" aria-hidden="true" />
            Reading the code… the first question on a repository can take a minute while it is indexed.
          </div>
        )}
        <div ref={endRef} />
      </div>

      {error && (
        <p className="chat__error" role="alert">
          {error}
        </p>
      )}

      <form className="chat__form" onSubmit={handleSubmit}>
        <label htmlFor={inputId} className="visually-hidden">
          Your question
        </label>
        <textarea
          id={inputId}
          className="input chat__input"
          rows={2}
          value={draft}
          maxLength={ASK_LIMITS.maxQuestion}
          placeholder="Ask a question…"
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={handleKeyDown}
          disabled={busy}
        />
        <button type="submit" className="button" disabled={busy || !draft.trim()}>
          Ask
        </button>
      </form>
    </section>
  );
}

function Sources({ sources }: { sources: AskSource[] }) {
  return (
    <ol className="chat__sources" aria-label="Sources">
      {sources.map((s) => (
        <li key={s.n}>
          <a href={s.url} target="_blank" rel="noopener noreferrer">
            [{s.n}] {s.path}:{s.startLine}-{s.endLine}
          </a>
          <details>
            <summary>Show code</summary>
            <pre>{s.snippet}</pre>
          </details>
        </li>
      ))}
    </ol>
  );
}
