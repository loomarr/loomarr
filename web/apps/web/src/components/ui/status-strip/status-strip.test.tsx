import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { StatusStrip, StatusStripAction } from "./status-strip";

describe("StatusStrip", () => {
  // One polite live region, so "All channels are playing" → "Something needs fixing" is heard
  // once without stealing focus.
  it("is a single status region carrying the headline and its items", () => {
    render(
      <StatusStrip
        tone="ok"
        title="All 6 channels are playing"
        items={[
          {
            id: "requests",
            tone: "attention",
            title: "3 requests need you",
            action: <StatusStripAction primary>Review</StatusStripAction>,
          },
          {
            id: "restart",
            tone: "notice",
            title: "Restart Loomarr to finish saving 2 settings",
            action: <StatusStripAction>Restart…</StatusStripAction>,
          },
        ]}
      />,
    );
    const strip = screen.getByRole("status");
    expect(strip).toHaveTextContent("All 6 channels are playing");
    expect(within(strip).getByRole("button", { name: "Review" })).toHaveClass("bg-primary");
    expect(within(strip).getByRole("button", { name: "Restart…" })).toHaveClass("border");
  });

  it("borders itself in on-air red only when something is failing", () => {
    const { rerender } = render(<StatusStrip tone="ok" title="All channels are playing" />);
    expect(screen.getByRole("status")).toHaveClass("border-static-700");
    rerender(<StatusStrip tone="error" title="Something needs fixing" />);
    expect(screen.getByRole("status")).toHaveClass("border-onair/40");
  });

  it("pulses its dot only while loading, and only with motion allowed", () => {
    const { container, rerender } = render(
      <StatusStrip tone="idle" loading title="Checking your channels…" />,
    );
    const dot = () => container.querySelector("[aria-hidden='true']");
    expect(dot()).toHaveClass("motion-safe:animate-[onair-pulse_1.2s_ease-in-out_infinite]");
    rerender(<StatusStrip tone="idle" title="Nothing on air yet" />);
    expect(dot()?.className).not.toMatch(/animate/);
  });
});
