import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { ApiProvider } from "../api/ApiProvider";
import { ApiError, type ApiClient } from "../api/client";
import { createMockClient } from "../api/mockClient";
import type { AskInput } from "../api/types";
import { ChatPanel } from "./ChatPanel";

const fast = () => createMockClient({ latencyMs: 0, stepDelayMs: 1 });

/** A client whose ask() is scripted and records what it was sent. */
function scripted(replies: Array<string | Error>) {
  const calls: AskInput[] = [];
  const client: ApiClient = {
    ...fast(),
    async ask(input) {
      calls.push(input);
      const next = replies.shift() ?? "ok";
      if (next instanceof Error) throw next;
      return { answer: next, repo: "acme/shop", indexedRef: "abc", sources: [] };
    },
  };
  return { client, calls };
}

function renderChat(client: ApiClient, props: Partial<React.ComponentProps<typeof ChatPanel>> = {}) {
  return render(
    <ApiProvider client={client}>
      <ChatPanel repo="acme/shop" subtitle="Repository acme/shop" suggestions={["What does this do?", "Where is main?"]} {...props} />
    </ApiProvider>,
  );
}

const box = () => screen.getByLabelText("Your question");

describe("ChatPanel", () => {
  it("offers suggested questions and sends one when clicked", async () => {
    const { client, calls } = scripted(["It does a thing."]);
    renderChat(client);

    await userEvent.click(screen.getByRole("button", { name: "What does this do?" }));

    expect(await screen.findByText("It does a thing.")).toBeInTheDocument();
    expect(calls[0]).toMatchObject({ repo: "acme/shop", question: "What does this do?", history: [] });
    expect(screen.queryByRole("button", { name: "Where is main?" })).not.toBeInTheDocument();
  });

  it("sends a typed question with Enter, but Shift+Enter makes a new line", async () => {
    const { client, calls } = scripted(["answer"]);
    renderChat(client);

    await userEvent.type(box(), "line one{Shift>}{Enter}{/Shift}line two");
    expect(calls).toHaveLength(0);
    expect(box()).toHaveValue("line one\nline two");

    await userEvent.type(box(), "{Enter}");
    expect(await screen.findByText("answer")).toBeInTheDocument();
    expect(calls[0]?.question).toBe("line one\nline two");
    expect(box()).toHaveValue("");
  });

  it("keeps the earlier turns as history for a follow-up", async () => {
    const { client, calls } = scripted(["first answer", "second answer"]);
    renderChat(client);

    await userEvent.type(box(), "first{Enter}");
    await screen.findByText("first answer");
    await userEvent.type(box(), "second{Enter}");
    await screen.findByText("second answer");

    expect(calls[1]?.history).toEqual([
      { role: "user", content: "first" },
      { role: "assistant", content: "first answer" },
    ]);
  });

  it("cannot send an empty question", async () => {
    renderChat(scripted([]).client);
    expect(screen.getByRole("button", { name: "Ask" })).toBeDisabled();
    await userEvent.type(box(), "   ");
    expect(screen.getByRole("button", { name: "Ask" })).toBeDisabled();
  });

  it("shows an error and puts the question back so it can be retried", async () => {
    const { client } = scripted([new ApiError("ai_busy", "The AI is busy right now. Please try again in a minute.", 429)]);
    renderChat(client);

    await userEvent.type(box(), "my question{Enter}");

    expect(await screen.findByRole("alert")).toHaveTextContent("The AI is busy right now");
    expect(box()).toHaveValue("my question");
    expect(screen.queryByText("my question", { selector: "p" })).not.toBeInTheDocument();
  });

  it("lists the sources of an answer as links to GitHub with the code behind a toggle", async () => {
    const client: ApiClient = {
      ...fast(),
      ask: async () => ({
        answer: "It retries [1].",
        repo: "acme/shop",
        indexedRef: "abc",
        sources: [{ n: 1, path: "retry.go", startLine: 3, endLine: 9, url: "https://github.com/acme/shop/blob/abc/retry.go#L3-L9", snippet: "func Retry() {}" }],
      }),
    };
    renderChat(client);
    await userEvent.type(box(), "how?{Enter}");

    const sources = await screen.findByRole("list", { name: "Sources" });
    const link = within(sources).getByRole("link", { name: "[1] retry.go:3-9" });
    expect(link).toHaveAttribute("href", "https://github.com/acme/shop/blob/abc/retry.go#L3-L9");
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", expect.stringContaining("noopener"));
    expect(within(sources).getByText("func Retry() {}")).toBeInTheDocument();
  });

  it("asks about a review when given a review id instead of a repository", async () => {
    const { client, calls } = scripted(["ok"]);
    renderChat(client, { repo: undefined, reviewId: "7" });
    await userEvent.type(box(), "q{Enter}");
    await screen.findByText("ok");
    expect(calls[0]).toMatchObject({ reviewId: "7", repo: undefined });
  });

  it("works against the demo client too", async () => {
    renderChat(fast());
    await userEvent.type(box(), "anything{Enter}");
    expect(await screen.findByText(/Demo mode: this answer is simulated/)).toBeInTheDocument();
  });
});
