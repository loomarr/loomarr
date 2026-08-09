import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { Button } from "../button";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "./tooltip";

// The icon-only-button label primitive (a themed wrapper over Base UI Tooltip).
//
// ⚠ DO NOT reach for `getByRole("tooltip")` here — it will never match, and that is not a bug in
// this wrapper. Base UI's tooltip is visual-only ON PURPOSE: the Popup carries no `role="tooltip"`
// and the Trigger gets no `aria-describedby`. Its docs are explicit — "Tooltips are visual-only
// elements and are not a replacement for labeling the trigger. The tooltip's trigger must have an
// `aria-label` attribute that closely matches the tooltip's content." Radix wired that association
// for us, so the swap removed it silently; the app's convention (every trigger carries its own
// `aria-label`) is what keeps the labels reachable, and `FieldHelp` — the one place the content is
// real information — declares an explicit description of its own.
//
// So what is pinned below is the VISUAL contract plus the composition, which is what this wrapper
// actually owns: closed until interacted with, opens on hover AND on keyboard focus, and `render`
// merges onto the app Button rather than nesting a second one.
const Labelled = () => (
  <TooltipProvider delay={0}>
    <Tooltip>
      <TooltipTrigger render={<Button variant="ghost" size="icon" aria-label="Reconcile now" />}>
        ↻
      </TooltipTrigger>
      <TooltipContent>Reconcile now</TooltipContent>
    </Tooltip>
  </TooltipProvider>
);

describe("Tooltip", () => {
  it("does not render its content until the trigger is interacted with", () => {
    render(<Labelled />);

    expect(screen.getByRole("button", { name: "Reconcile now" })).toBeInTheDocument();
    expect(screen.queryByText("Reconcile now", { selector: "div" })).not.toBeInTheDocument();
  });

  it("reveals the label on hover", async () => {
    render(<Labelled />);
    await userEvent.hover(screen.getByRole("button", { name: "Reconcile now" }));

    expect(await screen.findByText("Reconcile now", { selector: "div" })).toBeVisible();
  });

  // The property the wrapper exists for: the native `title=` it replaced is keyboard-hostile, so a
  // keyboard-only user getting no label would defeat the component. ⚠ axe cannot catch this — it
  // is an interaction, not markup.
  it("reveals the label on keyboard focus, not only on hover", async () => {
    render(<Labelled />);
    await userEvent.tab();

    expect(screen.getByRole("button", { name: "Reconcile now" })).toHaveFocus();
    expect(await screen.findByText("Reconcile now", { selector: "div" })).toBeVisible();
  });

  it("composes the trigger onto the app Button via render, without nesting a second button", () => {
    render(<Labelled />);

    // One button, carrying the Button primitive's own classes — proof the render element was
    // merged rather than wrapped. A nested <button> would make this query find two.
    const triggers = screen.getAllByRole("button", { name: "Reconcile now" });
    expect(triggers).toHaveLength(1);
    expect(triggers[0]).toHaveClass("inline-flex");
  });
});
