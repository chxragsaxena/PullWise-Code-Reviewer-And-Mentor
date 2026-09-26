import type { ReviewStatus } from "../api/types";

/** Steps shown in the progress view, in pipeline order. */
export const PROGRESS_STEPS = ["queued", "fetching_pr", "analyzing", "reviewing", "completed"] as const;

export const STATUS_LABEL: Record<ReviewStatus, string> = {
  queued: "Queued",
  fetching_pr: "Fetching pull request",
  analyzing: "Analyzing code",
  reviewing: "Reviewing in context",
  completed: "Completed",
  failed: "Failed",
};

/** A terminal status never changes again. */
export function isTerminal(status: ReviewStatus): boolean {
  return status === "completed" || status === "failed";
}
