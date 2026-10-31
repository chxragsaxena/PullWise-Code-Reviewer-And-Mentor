import { ApiError, type ApiClient, type ProgressHandlers } from "./client";
import {
  REVIEW_STATUSES,
  type AskAnswer,
  type AuthUser,
  type FindingDetail,
  type Review,
  type ReviewStatusEvent,
  type ReviewSummary,
  type Rule,
} from "./types";
import { isTerminal } from "../lib/status";
import { beginWaking } from "./waking";

/** Waits between retries while a sleeping server wakes up: about a minute in all. */
export const WAKE_RETRY_DELAYS_MS = [2000, 4000, 6000, 8000, 10000, 12000, 15000];

/** Network failures are retried less: offline is more likely than a cold start. */
const MAX_NETWORK_RETRIES = 2;

const GATEWAY_STATUSES = new Set([502, 503, 504]);

export interface HttpClientOptions {
  retryDelaysMs?: number[];
}

/** Talks to the Go backend described in docs/api.md. */
export function createHttpClient(baseUrl = "", { retryDelaysMs = WAKE_RETRY_DELAYS_MS }: HttpClientOptions = {}): ApiClient {
  async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
    const headers: Record<string, string> = { Accept: "application/json" };
    if (init.body !== undefined) headers["Content-Type"] = "application/json";

    let stopWaiting: (() => void) | null = null;
    try {
      for (let attempt = 0; ; attempt++) {
        let response: Response | null = null;
        try {
          // Same-origin in dev (Vite proxy) and prod, so the session cookie needs no CORS handling.
          response = await fetch(`${baseUrl}${path}`, { ...init, headers, credentials: "same-origin" });
        } catch {
          if (attempt >= Math.min(MAX_NETWORK_RETRIES, retryDelaysMs.length)) {
            throw new ApiError("network_error", "Could not reach the server. Check your connection and try again.", 0);
          }
        }

        if (response?.ok) {
          if (response.status === 204) return undefined as T;
          return (await response.json()) as T;
        }
        if (response && !(await isGatewayFailure(response))) throw await toApiError(response);
        if (attempt >= retryDelaysMs.length) {
          throw new ApiError(
            "server_unavailable",
            "The server is not responding. It may still be waking up, so please try again in a minute.",
            response?.status ?? 0,
          );
        }

        stopWaiting ??= beginWaking();
        await sleep(retryDelaysMs[attempt] ?? 0);
      }
    } finally {
      stopWaiting?.();
    }
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
      // A full navigation, not a fetch: GitHub needs the redirect chain and the cookie is HttpOnly.
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

    ask(input) {
      return request<AskAnswer>("/api/ask", { method: "POST", body: JSON.stringify(input) });
    },
  };
}

const sleep = (ms: number) => new Promise<void>((resolve) => setTimeout(resolve, ms));

/**
 * A gateway status without our JSON error body came from the proxy in front of a sleeping or restarting
 * server. The backend's own 502/503/504 errors carry a body and are real answers, so they are not retried.
 */
async function isGatewayFailure(response: Response): Promise<boolean> {
  if (!GATEWAY_STATUSES.has(response.status)) return false;
  try {
    const body = (await response.clone().json()) as { error?: { code?: string } };
    return !body.error?.code;
  } catch {
    return true;
  }
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
    // The server closes after a terminal status; close our side so the browser does not reconnect.
    if (isTerminal(event.status)) source.close();
  });

  // A browser reconnect is not an error; only a permanently closed stream is.
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
