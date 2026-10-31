import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { TryExamples } from "./TryExamples";

const EXAMPLES = [
  { id: "a", title: "Example A", detail: "first" },
  { id: "b", title: "Example B", detail: "second" },
];

describe("TryExamples", () => {
  it("lists the examples and reports which one was picked", async () => {
    const onPick = vi.fn();
    render(<TryExamples heading="Try one:" examples={EXAMPLES} onPick={onPick} />);

    expect(screen.getByText("Try one:")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /Example B/ }));
    expect(onPick).toHaveBeenCalledWith("b");
  });

  it("disables every example while one is starting", async () => {
    let finish: () => void = () => {};
    const onPick = vi.fn(() => new Promise<void>((resolve) => (finish = resolve)));
    render(<TryExamples heading="h" examples={EXAMPLES} onPick={onPick} />);

    await userEvent.click(screen.getByRole("button", { name: /Example A/ }));

    expect(screen.getByText("Starting…")).toBeInTheDocument();
    for (const button of screen.getAllByRole("button")) expect(button).toBeDisabled();
    finish();
  });

  it("shows the reason and re-enables the examples when starting fails", async () => {
    const onPick = vi.fn().mockRejectedValue(new Error("GitHub is busy"));
    render(<TryExamples heading="h" examples={EXAMPLES} onPick={onPick} />);

    await userEvent.click(screen.getByRole("button", { name: /Example A/ }));

    expect(await screen.findByRole("alert")).toHaveTextContent("GitHub is busy");
    for (const button of screen.getAllByRole("button")) expect(button).toBeEnabled();
  });
});
