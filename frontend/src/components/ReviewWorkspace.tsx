import { useState } from "react";
import type { Review } from "../api/types";
import { ChatPanel } from "./ChatPanel";
import { ReviewResults } from "./ReviewResults";
import "./ReviewWorkspace.css";

type Tab = "review" | "ask";

/** The review on the left and a chat about the pull request and its repository on the right. */
export function ReviewWorkspace({ review }: { review: Review }) {
  const [tab, setTab] = useState<Tab>("review");
  const { owner, repo } = review.pullRequest;

  const suggestions = [
    "Summarize what this pull request changes",
    "Which finding should I fix first, and why?",
    "Could this change break anything else in the repository?",
  ];
  const first = review.findings[0];
  if (first) suggestions.push(`Explain "${first.checkTitle}" in ${first.file}`);

  return (
    <div className="workspace" data-active={tab}>
      <div className="workspace__tabs" role="tablist" aria-label="Review and chat">
        <button type="button" role="tab" aria-selected={tab === "review"} onClick={() => setTab("review")}>
          Review
        </button>
        <button type="button" role="tab" aria-selected={tab === "ask"} onClick={() => setTab("ask")}>
          Ask
        </button>
      </div>

      <div className="workspace__review">
        <ReviewResults review={review} />
      </div>
      <aside className="workspace__chat">
        <ChatPanel
          reviewId={review.id}
          subtitle={`Pull request #${review.pullRequest.number} in ${owner}/${repo}`}
          suggestions={suggestions}
        />
      </aside>
    </div>
  );
}
