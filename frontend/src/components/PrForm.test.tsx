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

  it("offers the user's earlier repositories and fills in the link up to the pull request number", async () => {
    render(<PrForm onSubmit={vi.fn()} repos={["go-chi/chi", "acme/shop"]} />);

    await userEvent.click(screen.getByRole("button", { name: "acme/shop" }));

    expect(field()).toHaveValue("https://github.com/acme/shop/pull/");
    expect(field()).toHaveFocus();
    const options = Array.from(document.querySelectorAll("datalist option")).map((o) => o.getAttribute("value"));
    expect(options).toEqual(["https://github.com/go-chi/chi/pull/", "https://github.com/acme/shop/pull/"]);
    expect(field()).toHaveAttribute("list", document.querySelector("datalist")?.id);
  });

  it("shows at most five repositories as buttons", () => {
    const repos = ["a/1", "a/2", "a/3", "a/4", "a/5", "a/6", "a/7"];
    render(<PrForm onSubmit={vi.fn()} repos={repos} />);

    expect(screen.getAllByRole("button", { name: /^a\// })).toHaveLength(5);
    expect(document.querySelectorAll("datalist option")).toHaveLength(7);
  });

  it("shows no suggestions for someone with no history", () => {
    render(<PrForm onSubmit={vi.fn()} repos={[]} />);

    expect(screen.queryByText("Your repositories:")).not.toBeInTheDocument();
    expect(document.querySelector("datalist")).toBeNull();
    expect(field()).not.toHaveAttribute("list");
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
