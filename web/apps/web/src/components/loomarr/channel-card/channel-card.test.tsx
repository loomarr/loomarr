import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ChannelCard } from "./channel-card";

describe("ChannelCard", () => {
  it("shows the mono number, name, managed badge and now/next", () => {
    render(
      <ChannelCard
        number={42}
        name="90s Action"
        onAir="live"
        nowNext={{ now: { title: "Heat" }, next: { title: "Con Air" } }}
      />,
    );
    expect(screen.getByText("42")).toBeInTheDocument();
    expect(screen.getByText("90s Action")).toBeInTheDocument();
    expect(screen.getByText("Managed by Loomarr")).toBeInTheDocument();
    expect(screen.getByText("Heat")).toBeInTheDocument();
    expect(screen.getByText("On air")).toBeInTheDocument();
  });

  it("renders a health chip for non-healthy states only", () => {
    const { rerender } = render(<ChannelCard number={1} name="A" onAir="off" health="healthy" />);
    expect(screen.queryByText("Backfilling")).not.toBeInTheDocument();
    rerender(<ChannelCard number={1} name="A" onAir="reconciling" health="pending-slots" />);
    expect(screen.getByText("Backfilling")).toBeInTheDocument();
  });
});
