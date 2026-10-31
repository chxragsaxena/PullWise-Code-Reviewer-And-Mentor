import { act, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { beginWaking } from "../api/waking";
import { WakingBanner } from "./WakingBanner";

describe("WakingBanner", () => {
  it("appears while a request waits for the server and goes away afterwards", () => {
    render(<WakingBanner />);
    expect(screen.queryByRole("status")).not.toBeInTheDocument();

    let stop = () => {};
    act(() => {
      stop = beginWaking();
    });
    expect(screen.getByRole("status")).toHaveTextContent(/waking up/);

    act(() => stop());
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });

  it("stays while any request is still waiting", () => {
    render(<WakingBanner />);
    let stopA = () => {};
    let stopB = () => {};
    act(() => {
      stopA = beginWaking();
      stopB = beginWaking();
    });
    act(() => stopA());
    expect(screen.getByRole("status")).toBeInTheDocument();
    act(() => stopB());
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });
});
