import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { StatusDot } from "./status-dot";

describe("StatusDot", () => {
  it("exposes the label as an accessible name", () => {
    render(<StatusDot tone="ok" label="Healthy" />);
    expect(screen.getByRole("img", { name: "Healthy" })).toBeInTheDocument();
  });

  // An empty label is the documented opt-out for a dot sitting beside text that already
  // says the same thing — the Tasks page uses it on paused rows so a screen reader does
  // not announce "Paused" twice. It must drop the role too, or the dot is still a node in
  // the a11y tree with no name at all, which is worse than either alternative.
  it("is hidden from assistive tech when the label is empty", () => {
    const { container } = render(<StatusDot tone="off" label="" />);
    expect(screen.queryByRole("img")).not.toBeInTheDocument();
    expect(container.firstChild).toHaveAttribute("aria-hidden");
  });

  // The pulse is the dot's one piece of motion vocabulary and it belongs to `live` alone.
  // `error` shares the same colour deliberately, so this pair is what keeps them distinct:
  // if `error` ever picked up the animation, a job that failed hours ago would render as
  // something happening right now.
  it("animates only the live tone", () => {
    const { container: live } = render(<StatusDot tone="live" label="On air" />);
    expect(live.firstChild).toHaveClass("motion-safe:animate-pulse");

    const { container: failed } = render(<StatusDot tone="error" label="Failed" />);
    expect(failed.firstChild).toHaveClass("bg-onair");
    expect(failed.firstChild).not.toHaveClass("motion-safe:animate-pulse");
  });

  it("forwards span attributes the caller attaches", () => {
    render(<StatusDot tone="warn" label="Drifted" title="Drifted 4m" data-testid="dot" />);
    const dot = screen.getByTestId("dot");
    expect(dot).toHaveAttribute("title", "Drifted 4m");
  });
});
