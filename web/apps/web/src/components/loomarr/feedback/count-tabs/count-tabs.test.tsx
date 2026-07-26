import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { CountTabs } from "./count-tabs";

const tabs = [
  { id: "approval", label: "Needs approval", count: 3 },
  { id: "flight", label: "In flight", count: 12 },
  { id: "history", label: "History", count: 14 },
];

describe("CountTabs", () => {
  it("renders a tab per section with its count", () => {
    render(<CountTabs tabs={tabs} activeId="flight" onSelect={vi.fn()} label="Queue sections" />);

    expect(screen.getAllByRole("tab")).toHaveLength(3);
    expect(screen.getByRole("tab", { name: /Needs approval/ })).toHaveTextContent("3");
    expect(screen.getByRole("tab", { name: /History/ })).toHaveTextContent("14");
  });

  // A zero count is information ("nothing needs you"); an absent badge is ambiguous.
  it("shows a zero count rather than hiding it", () => {
    render(
      <CountTabs
        tabs={[{ id: "approval", label: "Needs approval", count: 0 }]}
        activeId="approval"
        onSelect={vi.fn()}
        label="Queue sections"
      />,
    );
    expect(screen.getByRole("tab", { name: /Needs approval/ })).toHaveTextContent("0");
  });

  it("marks the active tab selected and wires it to its panel", () => {
    render(<CountTabs tabs={tabs} activeId="history" onSelect={vi.fn()} label="Queue sections" />);

    const active = screen.getByRole("tab", { name: /History/ });
    expect(active).toHaveAttribute("aria-selected", "true");
    expect(active).toHaveAttribute("aria-controls", "panel-history");
    expect(screen.getByRole("tab", { name: /In flight/ })).toHaveAttribute("aria-selected", "false");
  });

  it("reports a click", async () => {
    const onSelect = vi.fn();
    render(<CountTabs tabs={tabs} activeId="flight" onSelect={onSelect} label="Queue sections" />);

    await userEvent.click(screen.getByRole("tab", { name: /History/ }));
    expect(onSelect).toHaveBeenCalledWith("history");
  });

  // The standard tablist pattern: only the active tab is tabbable, ←/→ move between them. A row
  // of buttons that all take tab stops makes a keyboard user walk every tab to leave the bar.
  it("puts only the active tab in the tab sequence", () => {
    render(<CountTabs tabs={tabs} activeId="flight" onSelect={vi.fn()} label="Queue sections" />);

    expect(screen.getByRole("tab", { name: /In flight/ })).toHaveAttribute("tabindex", "0");
    expect(screen.getByRole("tab", { name: /History/ })).toHaveAttribute("tabindex", "-1");
  });

  it("moves between tabs with the arrow keys, wrapping at both ends", async () => {
    const onSelect = vi.fn();
    render(<CountTabs tabs={tabs} activeId="flight" onSelect={onSelect} label="Queue sections" />);

    screen.getByRole("tab", { name: /In flight/ }).focus();
    await userEvent.keyboard("{ArrowRight}");
    expect(onSelect).toHaveBeenLastCalledWith("history");

    await userEvent.keyboard("{ArrowLeft}");
    expect(onSelect).toHaveBeenLastCalledWith("approval");
  });

  it("wraps forward from the last tab", async () => {
    const onSelect = vi.fn();
    render(<CountTabs tabs={tabs} activeId="history" onSelect={onSelect} label="Queue sections" />);

    screen.getByRole("tab", { name: /History/ }).focus();
    await userEvent.keyboard("{ArrowRight}");
    expect(onSelect).toHaveBeenLastCalledWith("approval");
  });

  it("names the tablist for screen readers", () => {
    render(<CountTabs tabs={tabs} activeId="flight" onSelect={vi.fn()} label="Queue sections" />);
    expect(screen.getByRole("tablist", { name: "Queue sections" })).toBeInTheDocument();
  });
});
