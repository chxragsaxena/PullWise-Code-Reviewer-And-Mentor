import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { PrForm } from "./PrForm";

const field = () => screen.getByLabelText("GitHub pull request link");
const submit = () => screen.getByRole("button", { name: "Review PR" });

describe("PrForm", () => {
  it("rejects a bad link without calling onSubmit", async () => {
    const onSubmit = vi.fn().mockResolvedValue(undefined);
    render(<PrForm onSubmit={onSubmit} />);

    await userEvent.type(field(), "https://gitlab.com/o/r/pull/1");
    await userEvent.click(submit());

    expect(await screen.findByRole("alert")).toHaveTextContent("Enter a GitHub pull request link");
    expect(field()).toHaveAttribute("aria-invalid", "true");
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("submits the trimmed link and disables the form", async () => {
    const onSubmit = vi.fn().mockResolvedValue(undefined);
    render(<PrForm onSubmit={onSubmit} />);

    await userEvent.type(field(), "  https://github.com/o/r/pull/7  ");
    await userEvent.click(submit());

    expect(onSubmit).toHaveBeenCalledWith("https://github.com/o/r/pull/7");
    expect(screen.getByRole("button", { name: "Starting…" })).toBeDisabled();
  });

  it("shows the error when starting the review fails and lets the user retry", async () => {
    const onSubmit = vi.fn().mockRejectedValueOnce(new Error("Server is down")).mockResolvedValue(undefined);
    render(<PrForm onSubmit={onSubmit} />);

    await userEvent.type(field(), "github.com/o/r/pull/7");
    await userEvent.click(submit());
    expect(await screen.findByRole("alert")).toHaveTextContent("Server is down");

    await userEvent.click(submit());
    expect(onSubmit).toHaveBeenCalledTimes(2);
  });
});
