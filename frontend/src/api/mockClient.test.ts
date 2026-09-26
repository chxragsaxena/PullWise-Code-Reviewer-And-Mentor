import { describe, expect, it } from "vitest";
import { ApiError } from "./client";
import { FAILING_PR_NUMBER, createMockClient } from "./mockClient";
import type { ReviewStatusEvent } from "./types";

const fast = () => createMockClient({ latencyMs: 0, stepDelayMs: 1 });

function collectUntilTerminal(client: ReturnType<typeof fast>, id: string): Promise<ReviewStatusEvent[]> {
  return new Promise((resolve, reject) => {
    const events: ReviewStatusEvent[] = [];
    const stop = client.subscribeToReview(id, {
      onEvent(event) {
        events.push(event);
        if (event.status === "completed" || event.status === "failed") {
          stop();
          resolve(events);
        }
      },
      onError: reject,
    });
  });
}

describe("mock API client", () => {
  it("rejects an invalid PR link with the documented error code", async () => {
    const error = await fast()
      .createReview("https://example.com/not-a-pr")
      .catch((e: unknown) => e);
    expect(error).toBeInstanceOf(ApiError);
    expect(error).toMatchObject({ code: "invalid_pr_url", status: 400 });
  });

  it("starts a review as queued and streams it through the pipeline to completion", async () => {
    const client = fast();
    const created = await client.createReview("https://github.com/o/r/pull/7");
    expect(created.status).toBe("queued");
    expect(created.counts).toEqual({ critical: 0, high: 0, medium: 0, suggestion: 0 });

    const events = await collectUntilTerminal(client, created.id);
    expect(events.map((e) => e.status)).toEqual([
      "queued",
      "fetching_pr",
      "analyzing",
      "reviewing",
      "completed",
    ]);

    const review = await client.getReview(created.id);
    expect(review.status).toBe("completed");
    expect(review.findings.length).toBeGreaterThan(0);
    expect(review.completedAt).not.toBeNull();
  });

  it("fails a review for the demo failing PR number", async () => {
    const client = fast();
    const created = await client.createReview(`https://github.com/o/r/pull/${FAILING_PR_NUMBER}`);
    const events = await collectUntilTerminal(client, created.id);
    expect(events.at(-1)?.status).toBe("failed");

    const review = await client.getReview(created.id);
    expect(review.error).toMatchObject({ code: "pr_not_found", message: expect.stringMatching(/not found/i) });
    expect(review.findings).toEqual([]);
  });

  it("lists seeded reviews newest first, including new ones", async () => {
    const client = fast();
    const before = await client.listReviews();
    expect(before.length).toBeGreaterThanOrEqual(3);

    const created = await client.createReview("https://github.com/o/r/pull/9");
    const after = await client.listReviews();
    expect(after[0]?.id).toBe(created.id);
    const dates = after.map((r) => r.createdAt);
    expect([...dates].sort().reverse()).toEqual(dates);
  });

  it("returns a finding with its lesson, and 404s for unknown ids", async () => {
    const client = fast();
    const [first] = await client.listReviews();
    const review = await client.getReview(first!.id);
    const finding = review.findings[0]!;

    const detail = await client.getFinding(review.id, finding.id);
    expect(detail.id).toBe(finding.id);
    expect(detail.lesson?.markdown).toContain("##");

    await expect(client.getFinding(review.id, "nope")).rejects.toMatchObject({ code: "finding_not_found" });
    await expect(client.getReview("nope")).rejects.toMatchObject({ code: "review_not_found" });
  });

  it("reports an unknown review on the progress stream", async () => {
    const error = await new Promise<Error>((resolve) => {
      fast().subscribeToReview("nope", { onEvent: () => {}, onError: resolve });
    });
    expect(error).toMatchObject({ code: "review_not_found" });
  });

  it("stops delivering events after unsubscribe", async () => {
    const client = fast();
    const created = await client.createReview("https://github.com/o/r/pull/11");
    const received: string[] = [];
    const stop = client.subscribeToReview(created.id, {
      onEvent: (e) => received.push(e.status),
      onError: () => {},
    });
    await new Promise((r) => setTimeout(r, 0));
    stop();
    const countAtStop = received.length;
    await new Promise((r) => setTimeout(r, 20));
    expect(received).toHaveLength(countAtStop);
  });
});
