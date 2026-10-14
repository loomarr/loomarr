import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { TooltipProvider } from "@/components/ui";
import { FieldHelp } from "./field-help";

// The tooltip content lives in a portal and only renders on hover — not worth asserting in jsdom.
// What matters for a11y is that the trigger is a reachable button that both NAMES the field it
// explains and CARRIES its guidance as an accessible description.
//
// ⚠ The description assertion is a regression guard, not a nicety. Base UI's Tooltip is
// visual-only by design — no `role="tooltip"`, no `aria-describedby` on the trigger — where Radix
// wired that association for us. FieldHelp is the one place in the app where the tooltip's content
// is information rather than a restatement of the label (it renders `entry.doc` for every
// setting), so without an explicit description a screen-reader user gets the field's name and
// none of its documentation. If someone "simplifies" the sr-only span away, this goes red.
describe("FieldHelp", () => {
  it("renders a labelled help trigger for the field", () => {
    render(
      <TooltipProvider>
        <FieldHelp label="Ordering">How programs are sequenced.</FieldHelp>
      </TooltipProvider>,
    );
    expect(screen.getByRole("button", { name: /about ordering/i })).toBeInTheDocument();
  });

  it("exposes the guidance as the trigger's accessible description", () => {
    render(
      <TooltipProvider>
        <FieldHelp label="Ordering">How programs are sequenced.</FieldHelp>
      </TooltipProvider>,
    );
    expect(screen.getByRole("button", { name: /about ordering/i })).toHaveAccessibleDescription(
      "How programs are sequenced.",
    );
  });
});
