import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { App } from "../App";
import { ApiProvider } from "../api/ApiProvider";
import type { ApiClient } from "../api/client";
import { FAILING_PR_NUMBER, createMockClient } from "../api/mockClient";

const fastClient = () => createMockClient({ latencyMs: 0, stepDelayMs: 5 });

function renderApp(client: ApiClient, path = "/") {
  return render(
    <ApiProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <App />
      </MemoryRouter>
    </ApiProvider>,
  );
}

/** A seeded review that has at least one critical finding, so filtering is observable. */
async function reviewWithCritical(client: ApiClient) {
  for (const summary of await client.listReviews()) {
    const review = await client.getReview(summary.id);
    if (review.counts.critical > 0) return review;
  }
  throw new Error("no seeded review has a critical finding");
}

describe("Home page", () => {
  it("lists recent reviews and labels the demo mode", async () => {
    renderApp(fastClient());

    expect(await screen.findByText("acme/orders-service")).toBeInTheDocument();
    expect(screen.getByText(/Demo mode/)).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Recent reviews" })).toBeInTheDocument();
  });

  it("takes a PR link through progress to results", async () => {
    renderApp(fastClient());

    await userEvent.type(await screen.findByLabelText("GitHub pull request link"), "https://github.com/o/r/pull/7");
    await userEvent.click(screen.getByRole("button", { name: "Review PR" }));

    expect(await screen.findByRole("heading", { name: "Reviewing your pull request" })).toBeInTheDocument();
    expect(await screen.findByText(/^Showing \d+ of \d+ findings?$/, {}, { timeout: 3000 })).toBeInTheDocument();
    expect(screen.getByRole("group", { name: "Filter findings by severity" })).toBeInTheDocument();
  });
});

describe("Home page suggestions", () => {
  it("offers a signed-in user's earlier repositories and fills in the start of the link", async () => {
    const client = fastClient();
    await client.login();
    renderApp(client);

    const chip = await screen.findByRole("button", { name: "golang/example" });
    await userEvent.click(chip);

    expect(screen.getByLabelText("GitHub pull request link")).toHaveValue("https://github.com/golang/example/pull/");
  });

  it("offers nothing to someone signed out", async () => {
    renderApp(fastClient());
    await screen.findByText("acme/orders-service");
    expect(screen.queryByText("Your repositories:")).not.toBeInTheDocument();
  });
});

describe("Home page examples", () => {
  it("starts a review of a real pull request with one click, no link needed", async () => {
    renderApp(fastClient());

    await userEvent.click(await screen.findByRole("button", { name: /Go · gin #4852/ }));

    expect(await screen.findByRole("heading", { name: "Reviewing your pull request" })).toBeInTheDocument();
    expect(await screen.findByText(/^Showing \d+ of \d+ findings?$/, {}, { timeout: 3000 })).toBeInTheDocument();
  });
});

describe("Review page", () => {
  it("shows a clear failure state", async () => {
    const client = fastClient();
    const created = await client.createReview(`https://github.com/o/r/pull/${FAILING_PR_NUMBER}`);
    renderApp(client, `/reviews/${created.id}`);

    expect(await screen.findByText("The review could not be completed", {}, { timeout: 3000 })).toBeInTheDocument();
    expect(screen.getByText(/not found, or it is in a private repository/i)).toBeInTheDocument();
  });

  it("filters findings by severity and clears the filter", async () => {
    const client = fastClient();
    const review = await reviewWithCritical(client);
    const total = review.findings.length;
    renderApp(client, `/reviews/${review.id}`);

    expect(await screen.findByText(`Showing ${total} of ${total} ${total === 1 ? "finding" : "findings"}`)).toBeInTheDocument();

    await userEvent.click(within(screen.getByRole("group", { name: "Filter findings by severity" })).getByRole("button", { name: /Critical/ }));
    const critical = review.counts.critical;
    expect(screen.getByText(`Showing ${critical} of ${total} ${total === 1 ? "finding" : "findings"}`)).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Clear filters" }));
    expect(screen.getByText(`Showing ${total} of ${total} ${total === 1 ? "finding" : "findings"}`)).toBeInTheDocument();
  });

  it("offers the skill filter grouped as Language then Design, with counts", async () => {
    const client = fastClient();
    const review = await reviewWithCritical(client);
    renderApp(client, `/reviews/${review.id}`);

    const menu = await screen.findByRole("combobox", { name: "Skill" });
    expect(within(menu).getByRole("option", { name: `All skills (${review.findings.length})` })).toBeInTheDocument();
    // Language skills are listed before design skills.
    expect(within(menu).getAllByRole("group")[0]).toHaveAttribute("label", "Language");
    for (const option of within(menu).getAllByRole("option").slice(1)) {
      expect(option.textContent).toMatch(/\(\d+\)$/);
    }
  });

  it("puts the findings on the left and a chat about the pull request beside them", async () => {
    const client = fastClient();
    const review = await reviewWithCritical(client);
    renderApp(client, `/reviews/${review.id}`);

    const filters = await screen.findByRole("group", { name: "Filter findings by severity" });
    const chat = screen.getByRole("region", { name: "Ask about the code" });
    expect(filters.compareDocumentPosition(chat) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(within(chat).getByText(`Pull request #${review.pullRequest.number} in ${review.pullRequest.owner}/${review.pullRequest.repo}`)).toBeInTheDocument();
    expect(within(chat).getByRole("button", { name: "Summarize what this pull request changes" })).toBeInTheDocument();
  });

  it("answers a question about the review without leaving the findings", async () => {
    const client = fastClient();
    const review = await reviewWithCritical(client);
    renderApp(client, `/reviews/${review.id}`);
    await screen.findByRole("group", { name: "Filter findings by severity" });

    await userEvent.type(screen.getByLabelText("Your question"), "Is this safe?{Enter}");

    expect(await screen.findByText(/Demo mode: this answer is simulated/)).toBeInTheDocument();
    expect(screen.getByRole("group", { name: "Filter findings by severity" })).toBeInTheDocument();
  });

  it("says when the review does not exist", async () => {
    renderApp(fastClient(), "/reviews/nope");
    expect(await screen.findByText("Review not found")).toBeInTheDocument();
  });
});

describe("Finding page", () => {
  it("teaches: evidence, explanation, improvement and the lesson", async () => {
    const client = fastClient();
    const review = await reviewWithCritical(client);
    const finding = review.findings[0]!;
    renderApp(client, `/reviews/${review.id}/findings/${finding.id}`);

    expect(await screen.findByRole("heading", { name: finding.checkTitle })).toBeInTheDocument();
    for (const heading of ["Evidence", "What we found", "Why this matters", "Possible improvement", "Learn"]) {
      expect(screen.getByRole("heading", { name: heading })).toBeInTheDocument();
    }
    expect(screen.getByText(finding.why)).toBeInTheDocument();
    expect(screen.getByText(/Try it yourself/)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "← Back to review" })).toHaveAttribute("href", `/reviews/${review.id}`);
  });

  it("says when the finding does not exist", async () => {
    const client = fastClient();
    const [first] = await client.listReviews();
    renderApp(client, `/reviews/${first!.id}/findings/nope`);
    expect(await screen.findByText("Finding not found")).toBeInTheDocument();
  });
});
