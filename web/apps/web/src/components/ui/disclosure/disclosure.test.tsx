import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { Button } from "../button";
import { Disclosure } from "./disclosure";

const Row = ({ ...props }: { defaultOpen?: boolean; onOpenChange?: (open: boolean) => void }) => (
  <Disclosure {...props}>
    <div>
      <Disclosure.Trigger label="Show Archive.org collections" />
      <span>Archive.org</span>
      <Button size="sm">Scan</Button>
    </div>
    <Disclosure.Panel>
      <span>Three collection rows</span>
    </Disclosure.Panel>
  </Disclosure>
);

describe("Disclosure", () => {
  // ⚠ The trigger renders a bare chevron, so without the required `label` it would be a button
  // called "button" — a row of those is a list of nothing.
  it("names its trigger from the required label", () => {
    render(<Row />);

    expect(screen.getByRole("button", { name: "Show Archive.org collections" })).toBeInTheDocument();
  });

  // ⚠ THE reason this exists instead of CollapsibleSection, which wraps its whole header in one
  // <button>. A row that also holds a Scan button would be nesting interactive content inside a
  // button: unreachable by keyboard and undefined in the accessibility tree.
  it("leaves the rest of the row independently reachable", () => {
    render(<Row />);

    const scan = screen.getByRole("button", { name: "Scan" });
    const trigger = screen.getByRole("button", { name: "Show Archive.org collections" });
    expect(scan).toBeInTheDocument();
    expect(trigger).not.toContainElement(scan);
  });

  // ⚠ Closed content must LEAVE the accessibility tree, not merely be clipped — asserted by role
  // rather than by text, because `hiddenUntilFound` deliberately keeps the panel in the DOM so
  // find-in-page can reach it. A text query would pass over a panel that never closed.
  it("keeps a closed panel out of the accessibility tree, and opens it on click", async () => {
    render(<Row />);

    expect(screen.queryByText("Three collection rows")).not.toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "Show Archive.org collections" }));
    expect(screen.getByText("Three collection rows")).toBeVisible();
  });

  it("reports the state change to its caller", async () => {
    const onOpenChange = vi.fn();
    render(<Row onOpenChange={onOpenChange} />);

    await userEvent.click(screen.getByRole("button", { name: "Show Archive.org collections" }));

    // ⚠ One argument. Base UI passes (open, eventDetails); the public contract is (open) => void,
    // and widening it here would leak the vendor's shape into every call site.
    expect(onOpenChange).toHaveBeenCalledWith(true);
  });

  it("opens from the start when asked", () => {
    render(<Row defaultOpen />);

    expect(screen.getByText("Three collection rows")).toBeVisible();
  });
});
