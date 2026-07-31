import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Caption } from "./caption";

describe("Caption", () => {
  // text-2xs (11px) is the sanctioned caption step. The 21 hand-rolled copies this replaced
  // had drifted to 10px, 10.5px and 11px because the type scale bottomed out at 12px and each
  // component invented its own smaller value — so the size is the contract, not a detail.
  it("renders mono at the caption step", () => {
    render(<Caption>7:30 PM</Caption>);
    expect(screen.getByText("7:30 PM")).toHaveClass("font-mono", "text-2xs");
  });

  it("is muted by default and strong on request", () => {
    const { rerender } = render(<Caption>ch 4</Caption>);
    expect(screen.getByText("ch 4")).toHaveClass("text-static-400");

    rerender(<Caption tone="strong">ch 4</Caption>);
    expect(screen.getByText("ch 4")).toHaveClass("text-static-100");
  });

  // `shout` is the section-label voice ("POD · 1:10"); plain is metadata sitting quietly
  // beside content. Uppercase must not leak into the default, or every duration and clock
  // time in the app starts announcing itself.
  it("only uppercases when shouting", () => {
    const { rerender } = render(<Caption>psa</Caption>);
    expect(screen.getByText("psa")).not.toHaveClass("uppercase");

    rerender(<Caption shout>psa</Caption>);
    expect(screen.getByText("psa")).toHaveClass("uppercase", "tracking-wide");
  });

  // Callers reach for `as` when the caption is a <p> in a stack or a <dt> in a list. Without
  // it they fall back to hand-rolled markup, which is the duplication this component exists
  // to end.
  it("renders as the requested element", () => {
    render(<Caption as="p">Managed by Loomarr</Caption>);
    expect(screen.getByText("Managed by Loomarr").tagName).toBe("P");
  });

  // Toolbar labels override `shout`'s tracking-wide with a wider custom value, so a caller's
  // className has to win over the variant classes rather than merely joining them.
  it("lets a caller's className override the variant", () => {
    render(<Caption shout className="tracking-[0.06em]" data-testid="c" />);
    expect(screen.getByTestId("c")).toHaveClass("tracking-[0.06em]");
  });
});
