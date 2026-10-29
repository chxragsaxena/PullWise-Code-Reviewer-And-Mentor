import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { ApiProvider } from "../api/ApiProvider";
import { createMockClient } from "../api/mockClient";
import { AuthStatus } from "./AuthStatus";

const fastClient = () => createMockClient({ latencyMs: 0, stepDelayMs: 1 });

function renderAuthStatus(client = fastClient()) {
  render(
    <ApiProvider client={client}>
      <MemoryRouter>
        <AuthStatus />
      </MemoryRouter>
    </ApiProvider>,
  );
  return client;
}

describe("AuthStatus", () => {
  it("shows a sign-in button when signed out", async () => {
    renderAuthStatus();
    expect(await screen.findByRole("button", { name: "Sign in with GitHub" })).toBeInTheDocument();
  });

  it("signs in and shows the user, then signs out again", async () => {
    renderAuthStatus();

    await userEvent.click(await screen.findByRole("button", { name: "Sign in with GitHub" }));
    expect(await screen.findByText("octocat")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Sign out" })).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Sign out" }));
    expect(await screen.findByRole("button", { name: "Sign in with GitHub" })).toBeInTheDocument();
  });

  it("links to the standards page only when signed in", async () => {
    renderAuthStatus();
    await screen.findByRole("button", { name: "Sign in with GitHub" });
    expect(screen.queryByRole("link", { name: "Standards" })).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Sign in with GitHub" }));
    expect(await screen.findByRole("link", { name: "Standards" })).toHaveAttribute("href", "/standards");
  });

  it("reflects a user already signed in on mount", async () => {
    const client = fastClient();
    await client.login();
    renderAuthStatus(client);

    expect(await screen.findByText("octocat")).toBeInTheDocument();
  });
});
