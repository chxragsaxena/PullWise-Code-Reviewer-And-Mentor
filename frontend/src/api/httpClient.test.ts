import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "./client";
import { createHttpClient } from "./httpClient";
import { isWaking } from "./waking";

const gateway = (status = 502) => new Response("<html>Bad Gateway</html>", { status, headers: { "Content-Type": "text/html" } });
const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });

function stubFetch(...responses: Array<Response | Error>) {
  const fetchMock = vi.fn();
  for (const r of responses) {
    if (r instanceof Error) fetchMock.mockRejectedValueOnce(r);
    else fetchMock.mockResolvedValueOnce(r);
  }
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

const quick = () => createHttpClient("", { retryDelaysMs: [0, 0, 0] });

afterEach(() => vi.unstubAllGlobals());

describe("http client and a sleeping server", () => {
  it("retries a gateway error until the server wakes up", async () => {
    const fetchMock = stubFetch(gateway(502), gateway(504), json({ reviews: [] }));

    await expect(quick().listReviews()).resolves.toEqual([]);

    expect(fetchMock).toHaveBeenCalledTimes(3);
    expect(isWaking()).toBe(false);
  });

  it("reports waking while it waits and clears it afterwards", async () => {
    let wakingDuringRetry = false;
    const fetchMock = vi.fn().mockImplementation(async () => {
      if (fetchMock.mock.calls.length === 2) wakingDuringRetry = isWaking();
      return fetchMock.mock.calls.length === 1 ? gateway(503) : json({ user: null });
    });
    vi.stubGlobal("fetch", fetchMock);

    await quick().getCurrentUser();

    expect(wakingDuringRetry).toBe(true);
    expect(isWaking()).toBe(false);
  });

  it("does not retry the backend's own errors, even with a 502 status", async () => {
    const fetchMock = stubFetch(json({ error: { code: "ask_failed", message: "Could not answer that right now." } }, 502));

    const err = await quick().ask({ question: "q", history: [] }).catch((e: unknown) => e);

    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ code: "ask_failed", status: 502 });
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(isWaking()).toBe(false);
  });

  it("does not retry ordinary client errors", async () => {
    const fetchMock = stubFetch(json({ error: { code: "review_not_found", message: "No such review." } }, 404));

    await expect(quick().getReview("9")).rejects.toMatchObject({ code: "review_not_found" });
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("gives a friendly error once the retries run out", async () => {
    const fetchMock = stubFetch(gateway(), gateway(), gateway(), gateway());

    const err = await quick().listReviews().catch((e: unknown) => e);

    expect(err).toMatchObject({ code: "server_unavailable", status: 502 });
    expect((err as Error).message).toMatch(/waking up/);
    expect(fetchMock).toHaveBeenCalledTimes(4);
    expect(isWaking()).toBe(false);
  });

  it("retries a dropped connection a couple of times, then reports the network error", async () => {
    const fetchMock = stubFetch(new TypeError("failed"), new TypeError("failed"), new TypeError("failed"), json({ reviews: [] }));

    await expect(quick().listReviews()).rejects.toMatchObject({ code: "network_error" });
    expect(fetchMock).toHaveBeenCalledTimes(3);
    expect(isWaking()).toBe(false);
  });

  it("recovers when the connection comes back on a retry", async () => {
    stubFetch(new TypeError("failed"), json({ reviews: [{ id: "1" }] }));
    await expect(quick().listReviews()).resolves.toHaveLength(1);
  });
});
