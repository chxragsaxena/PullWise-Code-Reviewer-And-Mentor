import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { App } from "../App";
import { ApiProvider } from "../api/ApiProvider";
import type { ApiClient } from "../api/client";
import { createMockClient } from "../api/mockClient";

const fast = () => createMockClient({ latencyMs: 0, stepDelayMs: 1 });

function renderStandards(client: ApiClient) {
  render(
    <ApiProvider client={client}>
      <MemoryRouter initialEntries={["/standards"]}>
        <App />
      </MemoryRouter>
    </ApiProvider>,
  );
}

async function signedInClient() {
  const client = fast();
  await client.login();
  return client;
}

const field = (name: string) => screen.getByLabelText(name);

async function addRule(title: string, instructions: string) {
  await userEvent.click(await screen.findByRole("button", { name: "Add a standard" }));
  await userEvent.type(field("Title"), title);
  await userEvent.type(field("Instructions"), instructions);
  await userEvent.click(screen.getByRole("button", { name: "Add standard" }));
}

describe("Standards page", () => {
  it("asks you to sign in when you are signed out", async () => {
    renderStandards(fast());
    expect(await screen.findByText("Sign in to write your own standards")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Add a standard" })).not.toBeInTheDocument();
  });

  it("starts empty and adds a standard", async () => {
    renderStandards(await signedInClient());
    expect(await screen.findByText("No standards yet")).toBeInTheDocument();

    await addRule("Wrap errors", "Return errors with fmt.Errorf and %w.");

    expect(await screen.findByText("Wrap errors")).toBeInTheDocument();
    expect(screen.getByText("Return errors with fmt.Errorf and %w.")).toBeInTheDocument();
    expect(screen.getByText("1 of 20 standards")).toBeInTheDocument();
    expect(screen.queryByLabelText("Title")).not.toBeInTheDocument();
  });

  it("refuses an empty title without calling the server", async () => {
    const client = await signedInClient();
    renderStandards(client);
    await userEvent.click(await screen.findByRole("button", { name: "Add a standard" }));
    await userEvent.click(screen.getByRole("button", { name: "Add standard" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("Give the standard a title and instructions.");
    expect(await client.listRules()).toHaveLength(0);
  });

  it("edits a standard", async () => {
    renderStandards(await signedInClient());
    await addRule("Wrap errors", "Use %w.");
    await userEvent.click(await screen.findByRole("button", { name: "Edit" }));

    await userEvent.clear(field("Title"));
    await userEvent.type(field("Title"), "Wrap every error");
    await userEvent.click(screen.getByRole("button", { name: "Save changes" }));

    expect(await screen.findByText("Wrap every error")).toBeInTheDocument();
    expect(screen.queryByText("Wrap errors")).not.toBeInTheDocument();
  });

  it("turns a standard off and on", async () => {
    renderStandards(await signedInClient());
    await addRule("Wrap errors", "Use %w.");

    await userEvent.click(await screen.findByRole("button", { name: "Turn off" }));
    expect(await screen.findByText("Off")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Turn on" }));
    expect(await screen.findByRole("button", { name: "Turn off" })).toBeInTheDocument();
  });

  it("asks for confirmation before deleting, and lets you back out", async () => {
    renderStandards(await signedInClient());
    await addRule("Wrap errors", "Use %w.");

    await userEvent.click(await screen.findByRole("button", { name: "Delete" }));
    await userEvent.click(screen.getByRole("button", { name: "Keep" }));
    expect(screen.getByText("Wrap errors")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Delete" }));
    await userEvent.click(screen.getByRole("button", { name: "Confirm delete" }));
    expect(await screen.findByText("No standards yet")).toBeInTheDocument();
  });

  it("stops you adding past the limit", async () => {
    const client = await signedInClient();
    for (let i = 0; i < 20; i++) {
      await client.createRule({ title: `Rule ${i}`, instructions: "x", severity: "medium", enabled: true });
    }
    renderStandards(client);

    expect(await screen.findByText("20 of 20 standards")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Add a standard" })).toBeDisabled();
  });

  it("shows the severity of each standard", async () => {
    const client = await signedInClient();
    await client.createRule({ title: "Never panic", instructions: "x", severity: "critical", enabled: true });
    renderStandards(client);

    const card = (await screen.findByText("Never panic")).closest("li")!;
    expect(within(card).getByText("Critical")).toBeInTheDocument();
  });
});
