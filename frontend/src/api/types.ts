// Shapes of the REST API. Keep in sync with docs/api.md.

export const REVIEW_STATUSES = [
  "queued",
  "fetching_pr",
  "analyzing",
  "reviewing",
  "completed",
  "failed",
] as const;
export type ReviewStatus = (typeof REVIEW_STATUSES)[number];

export const SEVERITIES = ["critical", "high", "medium", "suggestion"] as const;
export type Severity = (typeof SEVERITIES)[number];

export type FindingSource = "deterministic" | "ai";

/**
 * The error shape used by every failed request and by Review.error. Named
 * ApiErrorBody here because ApiError is the class thrown by the client.
 */
export interface ApiErrorBody {
  code: string;
  message: string;
}

export interface PullRequest {
  owner: string;
  repo: string;
  number: number;
  title: string;
  author: string;
  url: string;
  headSha: string;
  filesChanged: number;
  additions: number;
  deletions: number;
}

export type SeverityCounts = Record<Severity, number>;

export interface ReviewSummary {
  id: string;
  status: ReviewStatus;
  pullRequest: PullRequest;
  /** All zero until the review completes. */
  counts: SeverityCounts;
  createdAt: string;
  completedAt: string | null;
}

export interface Finding {
  id: string;
  skillId: string;
  skillName: string;
  checkId: string;
  checkTitle: string;
  source: FindingSource;
  severity: Severity;
  /** 0..1, always 1 for deterministic findings. */
  confidence: number;
  file: string;
  /** null when the finding applies to the whole file. */
  line: number | null;
  endLine: number | null;
  /** Code excerpt or measurement backing the finding. */
  evidence: string;
  found: string;
  why: string;
  improvement: string;
  lessonId: string | null;
}

export interface Review extends ReviewSummary {
  /** Set when status is "failed". */
  error: ApiErrorBody | null;
  /** Empty until the review completes. */
  findings: Finding[];
}

export interface Lesson {
  id: string;
  markdown: string;
}

export interface FindingDetail extends Finding {
  lesson: Lesson | null;
}

/** One message on the review progress stream. */
export interface ReviewStatusEvent {
  status: ReviewStatus;
  message: string;
  at: string;
}

/** The signed-in GitHub user, from GET /auth/me. */
export interface AuthUser {
  id: string;
  login: string;
  name: string;
  avatarUrl: string;
}
