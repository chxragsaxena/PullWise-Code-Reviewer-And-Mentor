import { useEffect, useState } from "react";
import { useApi } from "../api/ApiProvider";
import type { ReviewStatusEvent } from "../api/types";

export interface ReviewProgress {
  /** Distinct status changes in the order received. */
  events: ReviewStatusEvent[];
  latest: ReviewStatusEvent | null;
  error: Error | null;
}

/** Follows a review's progress stream while `enabled`. */
export function useReviewProgress(reviewId: string, enabled: boolean): ReviewProgress {
  const api = useApi();
  const [events, setEvents] = useState<ReviewStatusEvent[]>([]);
  const [error, setError] = useState<Error | null>(null);

  useEffect(() => {
    if (!enabled) return;
    setEvents([]);
    setError(null);

    return api.subscribeToReview(reviewId, {
      // A reconnect replays the current status; skip repeats.
      onEvent: (event) =>
        setEvents((prev) => (prev.at(-1)?.status === event.status ? prev : [...prev, event])),
      onError: setError,
    });
  }, [api, reviewId, enabled]);

  return { events, latest: events.at(-1) ?? null, error };
}
