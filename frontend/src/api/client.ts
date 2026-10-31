import type {
  AskAnswer,
  AskInput,
  AuthUser,
  FindingDetail,
  Review,
  ReviewStatusEvent,
  ReviewSummary,
  Rule,
  RuleInput,
} from "./types";

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

/** The UI depends only on this interface, so mock and HTTP clients are interchangeable. */
export interface ApiClient {
  /** "mock" clients simulate data in the browser; the UI labels them as a demo. */
  readonly kind: "mock" | "http";
  createReview(prUrl: string): Promise<ReviewSummary>;
  listReviews(): Promise<ReviewSummary[]>;
  getReview(id: string): Promise<Review>;
  getFinding(reviewId: string, findingId: string): Promise<FindingDetail>;
  /** Streams status changes (current status first); returns a function that stops the stream. */
  subscribeToReview(id: string, handlers: ProgressHandlers): () => void;

  /** The signed-in user, or null if signed out. Always resolves, never throws. */
  getCurrentUser(): Promise<AuthUser | null>;
  /** Starts GitHub sign-in; the http client navigates away, the mock simulates success. */
  login(): Promise<void>;
  logout(): Promise<void>;

  /** The signed-in user's own standards; rejects with a 401 ApiError when signed out. */
  listRules(): Promise<Rule[]>;
  createRule(input: RuleInput): Promise<Rule>;
  updateRule(id: string, input: RuleInput): Promise<Rule>;
  deleteRule(id: string): Promise<void>;

  /** Answers a question about a repository (or a review's repository) from its indexed code. */
  ask(input: AskInput): Promise<AskAnswer>;

  /** Repositories the signed-in user used before ("owner/name", newest first); empty when signed out. */
  listRecentRepos(): Promise<string[]>;
}
