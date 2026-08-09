import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { CollapsibleSection } from "./collapsible-section";

// The `.reveal` grid trick keeps the body in the DOM even when collapsed (it's clipped, not
// unmounted), so presence/absence assertions can't rely on the body being removed. Instead
// assert the header's aria-expanded + the reveal container's state attribute, which is what
// actually drives visibility — and the visible/toggle behavior the user experiences.
//
// ⚠ The closed marker is `data-closed`, NOT `data-open="false"` (changed in V50c). Base UI's
// Collapsible.Panel emits `data-open` VALUELESS when open and swaps to `data-closed` when shut,
// where the hand-rolled version wrote a stringified React boolean. styles.css matches both
// shapes on purpose — connection-block still passes the boolean form.
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
    expect(screen.getByText(bodyText).closest(".reveal")).toHaveAttribute("data-closed");

    await user.click(header);
    expect(header).toHaveAttribute("aria-expanded", "true");
    // ⚠ Asserted as the EMPTY STRING, which is what a valueless attribute reads back as. The
    // stylesheet's `[data-open=""]` selector depends on exactly this, so a change in Base UI's
    // emission would fail here rather than silently stop matching in CSS no test evaluates.
    expect(screen.getByText(bodyText).closest(".reveal")).toHaveAttribute("data-open", "");

    await user.click(header);
    expect(header).toHaveAttribute("aria-expanded", "false");
  });

  // The capability the V50c port was for: a closed section's text stays reachable by the
  // browser's find-in-page, which the old `overflow:hidden` clip made impossible.
  it("keeps a closed body findable in-page rather than merely clipped", () => {
    render(
      <CollapsibleSection title="Advanced">
        <p>{bodyText}</p>
      </CollapsibleSection>,
    );
    const panel = screen.getByText(bodyText).closest(".reveal");
    // Still mounted while closed — `hiddenUntilFound` forces keepMounted, so find-in-page has
    // something to match against.
    expect(panel).toBeInTheDocument();
    expect(panel).toHaveAttribute("hidden", "until-found");
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
