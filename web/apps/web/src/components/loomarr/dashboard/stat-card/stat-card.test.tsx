import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { StatCard } from "./stat-card";

describe("StatCard", () => {
  it("renders the label, value and note", () => {
    render(<StatCard label="Channels" value={12} note="3 need attention" />);

    expect(screen.getByText("Channels")).toBeInTheDocument();
    expect(screen.getByText("12")).toBeInTheDocument();
    expect(screen.getByText("3 need attention")).toBeInTheDocument();
  });

  // The value carries the tone; the label is Caption's muted mono either way. Tinting the label
  // too would make a red card shout twice and lose the number as the thing being read.
  it("tones the value, not the label", () => {
    render(<StatCard label="Failing" value="2" note="acquisitions stuck" tone="onair" />);

    expect(screen.getByText("2")).toHaveClass("text-onair-300");
    expect(screen.getByText("Failing")).toHaveClass("text-static-400");
  });

  it("is neutral without a tone", () => {
    render(<StatCard label="Clips" value={840} note="filler catalog" />);
    expect(screen.getByText("840")).toHaveClass("text-muted-foreground");
  });

  // A dashboard number is mono so a column of cards aligns and a changing value does not
  // reflow its own card — the same "it came from a machine" rule the label follows.
  it("renders the value in mono", () => {
    render(<StatCard label="Pending" value="7" note="awaiting approval" />);
    expect(screen.getByText("7")).toHaveClass("font-mono");
  });
});
