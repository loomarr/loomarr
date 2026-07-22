import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { TooltipProvider } from "@/components/ui";
import { FieldHelp } from "./field-help";

// The tooltip content lives in a Radix portal and only renders on hover — not worth asserting
// in jsdom. What matters for a11y is that the trigger is a reachable button with an accessible
// label naming the field it explains, so a keyboard/screen-reader user can find the help.
describe("FieldHelp", () => {
  it("renders a labelled help trigger for the field", () => {
    render(
      <TooltipProvider>
        <FieldHelp label="Ordering">How programs are sequenced.</FieldHelp>
      </TooltipProvider>,
    );
    expect(screen.getByRole("button", { name: /about ordering/i })).toBeInTheDocument();
  });
});
