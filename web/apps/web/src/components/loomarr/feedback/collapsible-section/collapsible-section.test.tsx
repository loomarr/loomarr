import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { CollapsibleSection } from "./collapsible-section";

// The `.reveal` grid trick keeps the body in the DOM even when collapsed (it's clipped, not
// unmounted), so presence/absence assertions can't rely on the body being removed. Instead
// assert the header's aria-expanded + the reveal container's data-open, which is what actually
// drives visibility — and the visible/toggle behavior the user experiences.
const bodyText = "the section body";

describe("CollapsibleSection", () => {
  it("starts collapsed and expands on click", async () => {
    const user = userEvent.setup();
    render(
      <CollapsibleSection title="Programming rules">
        <p>{bodyText}</p>
      </CollapsibleSection>,
    );

    const header = screen.getByRole("button", { name: /programming rules/i });
    expect(header).toHaveAttribute("aria-expanded", "false");
    // The reveal container is closed.
    expect(screen.getByText(bodyText).closest(".reveal")).toHaveAttribute("data-open", "false");

    await user.click(header);
    expect(header).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByText(bodyText).closest(".reveal")).toHaveAttribute("data-open", "true");

    await user.click(header);
    expect(header).toHaveAttribute("aria-expanded", "false");
  });

  it("honors defaultOpen", () => {
    render(
      <CollapsibleSection title="Lineup" defaultOpen>
        <p>{bodyText}</p>
      </CollapsibleSection>,
    );
    expect(screen.getByRole("button", { name: /lineup/i })).toHaveAttribute("aria-expanded", "true");
  });

  it("renders the description and a trailing slot in the header", () => {
    render(
      <CollapsibleSection title="Filler" description="Shape the ad breaks." trailing={<span>dirty</span>}>
        <p>{bodyText}</p>
      </CollapsibleSection>,
    );
    expect(screen.getByText("Shape the ad breaks.")).toBeInTheDocument();
    expect(screen.getByText("dirty")).toBeInTheDocument();
  });

  it("can be controlled by a parent", async () => {
    const user = userEvent.setup();
    const changes: boolean[] = [];
    render(
      <CollapsibleSection title="Advanced" open={false} onOpenChange={(o) => changes.push(o)}>
        <p>{bodyText}</p>
      </CollapsibleSection>,
    );
    const header = screen.getByRole("button", { name: /advanced/i });
    // Controlled: stays closed (parent didn't flip `open`) but reports the intent.
    await user.click(header);
    expect(changes).toEqual([true]);
    expect(header).toHaveAttribute("aria-expanded", "false");
  });
});
