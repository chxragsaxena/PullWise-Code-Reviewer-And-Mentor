import { ApiError, type ApiClient, type ProgressHandlers } from "./client";
import { REVIEW_STATUSES, type FindingDetail, type Review, type ReviewEvent, type ReviewSummary } from "./types";
import { isTerminal } from "../lib/status";

/** Talks to the Go backend described in docs/api.md. */
export function createHttpClient(baseUrl = ""): ApiClient {
  async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
    const headers: Record<string, string> = { Accept: "application/json" };
    if (init.body !== undefined) headers["Content-Type"] = "application/json";

    let response: Response;
    try {
      response = await fetch(`${baseUrl}${path}`, { ...init, headers });
    } catch {
      throw new ApiError("network_error", "Could not reach the server. Check your connection and try again.", 0);
    }
    if (!response.ok) throw await toApiError(response);
    return (await response.json()) as T;
  }

  return {
    kind: "http",

    createReview(prUrl) {
      return request<ReviewSummary>("/api/reviews", { method: "POST", body: JSON.stringify({ prUrl }) });
    },

    async listReviews() {
      const body = await request<{ reviews: ReviewSummary[] }>("/api/reviews");
      return body.reviews;
    },

    getReview(id) {
      return request<Review>(`/api/reviews/${encodeURIComponent(id)}`);
    },

    getFinding(reviewId, findingId) {
      return request<FindingDetail>(
        `/api/reviews/${encodeURIComponent(reviewId)}/findings/${encodeURIComponent(findingId)}`,
      );
    },

    subscribeToReview(id, handlers) {
      return openEventStream(`${baseUrl}/api/reviews/${encodeURIComponent(id)}/events`, handlers);
    },
  };
}

async function toApiError(response: Response): Promise<ApiError> {
  try {
    const body = (await response.json()) as { error?: { code?: string; message?: string } };
    if (body.error?.code && body.error.message) {
      return new ApiError(body.error.code, body.error.message, response.status);
    }
  } catch {
    // Not JSON; fall through to the generic error.
  }
  return new ApiError("unexpected_error", `The server responded with ${response.status}.`, response.status);
}

function openEventStream(url: string, { onEvent, onError }: ProgressHandlers): () => void {
  const source = new EventSource(url);

  source.addEventListener("status", (message) => {
    const event = parseEvent((message as MessageEvent<string>).data);
    if (!event) {
      onError(new Error("Received a malformed progress update."));
      return;
    }
    onEvent(event);
    // The server closes the stream after a terminal status; close our side too
    // so the browser does not reconnect.
    if (isTerminal(event.status)) source.close();
  });

  // While the browser is reconnecting there is nothing to report. Only a
  // permanently closed stream is an error.
  source.onerror = () => {
    if (source.readyState === EventSource.CLOSED) onError(new Error("Lost connection to the review stream."));
  };

  return () => source.close();
}

function parseEvent(data: string): ReviewEvent | null {
  try {
    const value = JSON.parse(data) as Partial<ReviewEvent>;
    if (
      typeof value.message === "string" &&
      typeof value.at === "string" &&
      REVIEW_STATUSES.includes(value.status as ReviewEvent["status"])
    ) {
      return value as ReviewEvent;
    }
  } catch {
    // Fall through.
  }
  return null;
}
