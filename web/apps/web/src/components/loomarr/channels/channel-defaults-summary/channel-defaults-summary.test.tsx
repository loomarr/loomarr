import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { eraDates } from "@/lib/era-dates";
import { ChannelDefaultsSummary } from "./channel-defaults-summary";

describe("ChannelDefaultsSummary", () => {
  it("counts no overrides on a fresh channel", () => {
    render(<ChannelDefaultsSummary policy={{}} />);
    expect(screen.getByText(/^0 of 11 fields are channel overrides\./)).toBeInTheDocument();
  });

  it("counts each overridden field, and says where the real defaults live", () => {
    render(
      <ChannelDefaultsSummary
        policy={{
          audience: { ceiling: "TV-14" },
          ordering: "shuffle",
          scope: { dates: eraDates({ from: 1990 }) },
        }}
      />,
    );
    const summary = screen.getByText(/^3 of 11 fields are channel overrides\./);
    expect(summary).toHaveTextContent("Most defaults here are Loomarr's built-in values");
    expect(summary).toHaveTextContent(
      "only Schedule horizon and Commercial-break frequency live on Settings",
    );
  });
});
