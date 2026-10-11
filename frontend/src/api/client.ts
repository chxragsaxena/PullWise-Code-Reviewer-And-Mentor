import type { AuthUser, FindingDetail, Review, ReviewStatusEvent, ReviewSummary } from "./types";

/** Error carrying the backend's stable error code (see docs/api.md). */
export class ApiError extends Error {
  readonly code: string;
  readonly status: number;

  constructor(code: string, message: string, status: number) {
    super(message);
    this.name = "ApiError";
    this.code = code;
    this.status = status;
  }
}

export interface ProgressHandlers {
  onEvent: (event: ReviewStatusEvent) => void;
  onError: (error: Error) => void;
}

/**
 * Everything the UI needs from the backend. The UI depends only on this
 * interface, so the mock and the real HTTP client are interchangeable.
 */
export interface ApiClient {
  /** "mock" clients simulate data in the browser; the UI labels them as a demo. */
  readonly kind: "mock" | "http";
  createReview(prUrl: string): Promise<ReviewSummary>;
  listReviews(): Promise<ReviewSummary[]>;
  getReview(id: string): Promise<Review>;
  getFinding(reviewId: string, findingId: string): Promise<FindingDetail>;
  /**
   * Streams status changes for a review. The current status is delivered first.
   * Returns a function that stops the stream.
   */
  subscribeToReview(id: string, handlers: ProgressHandlers): () => void;

  /** The signed-in user, or null if signed out. Always resolves, never throws. */
  getCurrentUser(): Promise<AuthUser | null>;
  /** Starts GitHub sign-in. The http client navigates the browser away; the mock client simulates success in place. */
  login(): Promise<void>;
  logout(): Promise<void>;
}
