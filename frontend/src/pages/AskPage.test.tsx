import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { App } from "../App";
import { ApiProvider } from "../api/ApiProvider";
import { createMockClient } from "../api/mockClient";

function renderAsk(path = "/ask") {
  render(
    <ApiProvider client={createMockClient({ latencyMs: 0, stepDelayMs: 1 })}>
      <MemoryRouter initialEntries={[path]}>
        <App />
      </MemoryRouter>
    </ApiProvider>,
  );
}

describe("Ask page", () => {
  it("starts with a repository form", () => {
    renderAsk();
    expect(screen.getByRole("heading", { name: "Ask about a repository" })).toBeInTheDocument();
    expect(screen.queryByLabelText("Your question")).not.toBeInTheDocument();
  });

  it("rejects something that is not a GitHub repository", async () => {
    renderAsk();
    await userEvent.type(screen.getByLabelText("GitHub repository"), "https://gitlab.com/a/b");
    await userEvent.click(screen.getByRole("button", { name: "Start asking" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("Enter a GitHub repository");
    expect(screen.queryByLabelText("Your question")).not.toBeInTheDocument();
  });

  it.each(["go-chi/chi", "https://github.com/go-chi/chi", "github.com/go-chi/chi.git"])("opens a chat for %s", async (input) => {
    renderAsk();
    await userEvent.type(screen.getByLabelText("GitHub repository"), input);
    await userEvent.click(screen.getByRole("button", { name: "Start asking" }));

    expect(await screen.findByRole("heading", { name: /Ask about go-chi\/chi/ })).toBeInTheDocument();
    expect(screen.getByLabelText("Your question")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "What does this repository do?" })).toBeInTheDocument();
  });

  it("opens a chat from an example repository with one click", async () => {
    renderAsk();
    await userEvent.click(screen.getByRole("button", { name: /pullwise-knowledge/ }));

    expect(await screen.findByRole("heading", { name: /Ask about chxragsaxena\/pullwise-knowledge/ })).toBeInTheDocument();
    expect(screen.getByLabelText("Your question")).toBeInTheDocument();
  });

  it("suggests the repositories a signed-in user used before", async () => {
    const client = createMockClient({ latencyMs: 0, stepDelayMs: 1 });
    await client.login();
    await client.ask({ repo: "acme/shop", question: "q", history: [] });
    render(
      <ApiProvider client={client}>
        <MemoryRouter initialEntries={["/ask"]}>
          <App />
        </MemoryRouter>
      </ApiProvider>,
    );

    expect(await screen.findByText("Your recent repositories:")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "acme/shop" }));

    expect(await screen.findByRole("heading", { name: /Ask about acme\/shop/ })).toBeInTheDocument();
  });

  it("shows no history to someone who is signed out", async () => {
    renderAsk();
    await screen.findByRole("heading", { name: "Ask about a repository" });
    expect(screen.queryByText("Your recent repositories:")).not.toBeInTheDocument();
  });

  it("opens straight into the chat from a shared link", () => {
    renderAsk("/ask?repo=golang/go");
    expect(screen.getByRole("heading", { name: /Ask about golang\/go/ })).toBeInTheDocument();
  });

  it("goes back to the form with Change repository", async () => {
    renderAsk("/ask?repo=golang/go");
    await userEvent.click(screen.getByRole("button", { name: "Change repository" }));
    expect(await screen.findByRole("heading", { name: "Ask about a repository" })).toBeInTheDocument();
  });

  it("is reachable from the header", async () => {
    renderAsk("/");
    await userEvent.click(await screen.findByRole("link", { name: "Ask" }));
    expect(await screen.findByRole("heading", { name: "Ask about a repository" })).toBeInTheDocument();
  });
});
