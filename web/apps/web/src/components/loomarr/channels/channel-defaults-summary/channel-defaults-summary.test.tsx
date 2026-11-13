import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { eraDates } from "@/lib/era-dates";
import { render } from "@/test/render";
import { ChannelDefaultsSummary } from "./channel-defaults-summary";

describe("ChannelDefaultsSummary", () => {
  it("says a fresh channel uses the defaults, and that a change stays on this channel", () => {
    render(<ChannelDefaultsSummary policy={{}} />);
    expect(
      screen.getByText(
        "This channel uses Loomarr's defaults. Anything you change here applies to this channel only.",
      ),
    ).toBeInTheDocument();
  });

  it("draws nothing once a field is changed: each changed field marks itself", () => {
    render(
      <ChannelDefaultsSummary
        policy={{
          audience: { ceiling: "TV-14" },
          ordering: "shuffle",
          scope: { dates: eraDates({ from: 1990 }) },
        }}
      />,
    );
    expect(screen.queryByText(/uses Loomarr's defaults/)).not.toBeInTheDocument();
  });
});
