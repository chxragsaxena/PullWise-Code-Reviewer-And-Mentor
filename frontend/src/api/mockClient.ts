import { ApiError, type ApiClient } from "./client";
import { LESSONS, buildFindings, buildPullRequest } from "./mockData";
import type {
  AuthUser,
  FindingDetail,
  Review,
  ReviewStatusEvent,
  ReviewStatus,
  ReviewSummary,
  Rule,
  RuleInput,
} from "./types";
import { RULE_LIMITS } from "./types";
import { countBySeverity } from "../lib/findings";
import { parsePrUrl } from "../lib/prUrl";

/** Signed in by createMockClient's login(), since there is no real GitHub to redirect to. */
const MOCK_USER: AuthUser = {
  id: "mock-1",
  login: "octocat",
  name: "The Octocat",
  avatarUrl: "https://avatars.githubusercontent.com/u/583231?v=4",
};

export interface MockClientOptions {
  /** Simulated network latency per request, in ms. */
  latencyMs?: number;
  /** Time each pipeline step takes, in ms. */
  stepDelayMs?: number;
}

/** PR number that makes a simulated review fail, to demo the error state. */
export const FAILING_PR_NUMBER = 404;

const PIPELINE: readonly { status: ReviewStatus; message: string }[] = [
  { status: "fetching_pr", message: "Fetching the pull request and its diff from GitHub" },
  { status: "analyzing", message: "Running static analysis on the changed files" },
  { status: "reviewing", message: "Reviewing the code in context" },
  { status: "completed", message: "Review complete" },
];

interface StoredReview {
  review: Review;
  latest: ReviewStatusEvent;
  listeners: Set<(event: ReviewStatusEvent) => void>;
}

const snapshot = <T>(value: T): T => JSON.parse(JSON.stringify(value)) as T;

function toSummary(review: Review): ReviewSummary {
  return {
    id: review.id,
    status: review.status,
    pullRequest: review.pullRequest,
    counts: review.counts,
    createdAt: review.createdAt,
    completedAt: review.completedAt,
  };
}

/**
 * An in-browser stand-in for the Go backend that follows docs/api.md,
 * including a simulated progress stream.
 */
export function createMockClient({ latencyMs = 250, stepDelayMs = 900 }: MockClientOptions = {}): ApiClient {
  const store = new Map<string, StoredReview>();
  let nextId = 1;
  let currentUser: AuthUser | null = null;
  let rules: Rule[] = [];
  let nextRuleId = 1;

  function signedIn() {
    if (!currentUser) throw new ApiError("unauthorized", "Sign in to manage your standards.", 401);
  }

  function validRule(input: RuleInput) {
    const { maxTitle, maxInstructions } = RULE_LIMITS;
    if (!input.title.trim() || input.title.length > maxTitle) {
      throw new ApiError("invalid_rule", `The title must be 1 to ${maxTitle} characters.`, 400);
    }
    if (!input.instructions.trim() || input.instructions.length > maxInstructions) {
      throw new ApiError("invalid_rule", `The instructions must be 1 to ${maxInstructions} characters.`, 400);
    }
  }

  function respond<T>(produce: () => T): Promise<T> {
    return new Promise((resolve, reject) => {
      setTimeout(() => {
        try {
          resolve(produce());
        } catch (error) {
          reject(error);
        }
      }, latencyMs);
    });
  }

  function publish(stored: StoredReview, status: ReviewStatus, message: string) {
    const { review } = stored;
    review.status = status;

    if (status === "completed") {
      review.findings = buildFindings(review.id, review.pullRequest.number);
      review.counts = countBySeverity(review.findings);
      review.completedAt = new Date().toISOString();
    } else if (status === "failed") {
      review.error = { code: "pr_not_found", message };
      review.completedAt = new Date().toISOString();
    }

    stored.latest = { status, message, at: new Date().toISOString() };
    for (const listener of stored.listeners) listener(stored.latest);
  }

  function run(stored: StoredReview, step: number) {
    setTimeout(() => {
      const next = PIPELINE[step];
      if (!next) return;

      if (next.status === "fetching_pr" && stored.review.pullRequest.number === FAILING_PR_NUMBER) {
        publish(stored, "failed", "Pull request not found, or it is in a private repository.");
        return;
      }
      publish(stored, next.status, next.message);
      run(stored, step + 1);
    }, stepDelayMs);
  }

  function create(prUrl: string, createdAt: Date, complete: boolean): StoredReview {
    const ref = parsePrUrl(prUrl);
    if (!ref) throw new ApiError("invalid_pr_url", "Not a valid GitHub pull request URL.", 400);

    const id = `rev_${nextId++}`;
    const review: Review = {
      id,
      status: "queued",
      pullRequest: buildPullRequest(ref.owner, ref.repo, ref.number),
      counts: { critical: 0, high: 0, medium: 0, suggestion: 0 },
      createdAt: createdAt.toISOString(),
      completedAt: null,
      error: null,
      findings: [],
    };
    const stored: StoredReview = {
      review,
      latest: { status: "queued", message: "Waiting to start", at: review.createdAt },
      listeners: new Set(),
    };
    store.set(id, stored);
    if (complete) publish(stored, "completed", "Review complete");
    return stored;
  }

  const minutesAgo = (minutes: number) => new Date(Date.now() - minutes * 60_000);
  create("https://github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/pull/12", minutesAgo(90), true);
  create("https://github.com/golang/example/pull/48", minutesAgo(60 * 26), true);
  create("https://github.com/acme/orders-service/pull/305", minutesAgo(60 * 24 * 3), true);

  function find(id: string): StoredReview {
    const stored = store.get(id);
    if (!stored) throw new ApiError("review_not_found", "That review does not exist.", 404);
    return stored;
  }

  return {
    kind: "mock",

    createReview(prUrl) {
      return respond(() => {
        const stored = create(prUrl, new Date(), false);
        run(stored, 0);
        return toSummary(stored.review);
      });
    },

    listReviews() {
      return respond(() =>
        [...store.values()]
          .map((stored) => toSummary(stored.review))
          .sort((a, b) => b.createdAt.localeCompare(a.createdAt)),
      );
    },

    getReview(id) {
      return respond(() => snapshot(find(id).review));
    },

    getFinding(reviewId, findingId) {
      return respond((): FindingDetail => {
        const finding = find(reviewId).review.findings.find((f) => f.id === findingId);
        if (!finding) throw new ApiError("finding_not_found", "That finding does not exist.", 404);
        const lesson = finding.lessonId ? (LESSONS[finding.lessonId] ?? null) : null;
        return snapshot({ ...finding, lesson });
      });
    },

    subscribeToReview(id, { onEvent, onError }) {
      const stored = store.get(id);
      if (!stored) {
        queueMicrotask(() => onError(new ApiError("review_not_found", "That review does not exist.", 404)));
        return () => {};
      }

      let active = true;
      const listener = (event: ReviewStatusEvent) => {
        if (active) onEvent(snapshot(event));
      };
      stored.listeners.add(listener);
      // Like the real stream, start with the current status.
      queueMicrotask(() => listener(stored.latest));

      return () => {
        active = false;
        stored.listeners.delete(listener);
      };
    },

    getCurrentUser() {
      return respond(() => currentUser);
    },

    login() {
      return respond(() => {
        currentUser = MOCK_USER;
      });
    },

    logout() {
      return respond(() => {
        currentUser = null;
        rules = [];
      });
    },

    listRules() {
      return respond(() => {
        signedIn();
        return snapshot(rules);
      });
    },

    createRule(input) {
      return respond(() => {
        signedIn();
        validRule(input);
        if (rules.length >= RULE_LIMITS.maxRules) {
          throw new ApiError("limit_reached", "You can have at most 20 standards. Delete one to add another.", 409);
        }
        const rule: Rule = { ...input, title: input.title.trim(), instructions: input.instructions.trim(), id: String(nextRuleId++) };
        rules.push(rule);
        return snapshot(rule);
      });
    },

    updateRule(id, input) {
      return respond(() => {
        signedIn();
        validRule(input);
        const index = rules.findIndex((r) => r.id === id);
        if (index < 0) throw new ApiError("rule_not_found", "That rule does not exist.", 404);
        rules[index] = { ...input, title: input.title.trim(), instructions: input.instructions.trim(), id };
        return snapshot(rules[index]!);
      });
    },

    deleteRule(id) {
      return respond(() => {
        signedIn();
        if (!rules.some((r) => r.id === id)) throw new ApiError("rule_not_found", "That rule does not exist.", 404);
        rules = rules.filter((r) => r.id !== id);
      });
    },
  };
}
