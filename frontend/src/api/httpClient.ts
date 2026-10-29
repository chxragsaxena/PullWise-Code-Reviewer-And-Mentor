import { ApiError, type ApiClient, type ProgressHandlers } from "./client";
import {
  REVIEW_STATUSES,
  type AuthUser,
  type FindingDetail,
  type Review,
  type ReviewStatusEvent,
  type ReviewSummary,
  type Rule,
} from "./types";
import { isTerminal } from "../lib/status";

/** Talks to the Go backend described in docs/api.md. */
export function createHttpClient(baseUrl = ""): ApiClient {
  async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
    const headers: Record<string, string> = { Accept: "application/json" };
    if (init.body !== undefined) headers["Content-Type"] = "application/json";

    let response: Response;
    try {
      // Same-origin in both dev (Vite proxies /auth and /api to the backend)
      // and prod, so the session cookie needs no cross-origin CORS handling.
      response = await fetch(`${baseUrl}${path}`, { ...init, headers, credentials: "same-origin" });
    } catch {
      throw new ApiError("network_error", "Could not reach the server. Check your connection and try again.", 0);
    }
    if (!response.ok) throw await toApiError(response);
    if (response.status === 204) return undefined as T;
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

    async getCurrentUser() {
      const body = await request<{ user: AuthUser | null }>("/auth/me");
      return body.user;
    },

    async login() {
      // A full browser navigation, not a fetch: GitHub needs to see the
      // actual redirect chain, and the response sets an HttpOnly cookie.
      window.location.href = `${baseUrl}/auth/github/login`;
    },

    async logout() {
      await request("/auth/logout", { method: "POST" });
    },

    async listRules() {
      const body = await request<{ rules: Rule[] }>("/api/rules");
      return body.rules;
    },

    createRule(input) {
      return request<Rule>("/api/rules", { method: "POST", body: JSON.stringify(input) });
    },

    updateRule(id, input) {
      return request<Rule>(`/api/rules/${encodeURIComponent(id)}`, { method: "PUT", body: JSON.stringify(input) });
    },

    async deleteRule(id) {
      await request(`/api/rules/${encodeURIComponent(id)}`, { method: "DELETE" });
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

function parseEvent(data: string): ReviewStatusEvent | null {
  try {
    const value = JSON.parse(data) as Partial<ReviewStatusEvent>;
    if (
      typeof value.message === "string" &&
      typeof value.at === "string" &&
      REVIEW_STATUSES.includes(value.status as ReviewStatusEvent["status"])
    ) {
      return value as ReviewStatusEvent;
    }
  } catch {
    // Fall through.
  }
  return null;
}
