import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { DiagnosticsSplitPane } from "./diagnostics-split-pane";

describe("DiagnosticsSplitPane", () => {
  it("resizes accessibly and remembers the detail width for this session", () => {
    render(
      <DiagnosticsSplitPane
        storageKey="test-details"
        primary={<p>Evidence list</p>}
        secondary={<p>Selected evidence</p>}
      />,
    );

    const separator = screen.getByRole("separator", { name: "Resize details" });
    expect(separator).toHaveAttribute("aria-valuenow", "352");
    fireEvent.keyDown(separator, { key: "ArrowLeft" });
    expect(separator).toHaveAttribute("aria-valuenow", "368");
    expect(sessionStorage.getItem("loomarr:test-details:width")).toBe("368");
    fireEvent.keyDown(separator, { key: "Home" });
    expect(separator).toHaveAttribute("aria-valuenow", "288");
  });

  it("collapses and restores the detail pane without removing the primary evidence", async () => {
    render(
      <DiagnosticsSplitPane
        storageKey="test-collapse"
        primary={<p>Evidence list</p>}
        secondary={<p>Selected evidence</p>}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: "Hide details" }));
    expect(screen.getByText("Evidence list")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Show details" })).toBeInTheDocument();
    expect(screen.getByTestId("diagnostics-secondary-pane")).toHaveClass("hidden");

    await userEvent.click(screen.getByRole("button", { name: "Show details" }));
    expect(screen.getByRole("separator", { name: "Resize details" })).toBeInTheDocument();
  });

  it("restores details when the caller selects new evidence", async () => {
    const view = render(
      <DiagnosticsSplitPane
        storageKey="test-selection"
        primary={<p>Evidence list</p>}
        secondary={<p>Selected evidence</p>}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Hide details" }));

    view.rerender(
      <DiagnosticsSplitPane
        storageKey="test-selection"
        revealKey="event-2"
        primary={<p>Evidence list</p>}
        secondary={<p>Selected evidence</p>}
      />,
    );

    expect(screen.getByRole("separator", { name: "Resize details" })).toBeInTheDocument();
  });
});
