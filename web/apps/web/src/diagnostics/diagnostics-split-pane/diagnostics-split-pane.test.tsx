import { fireEvent, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { render } from "@/test/render";
import { setWindowWidth } from "@/test/window-width";
import { DiagnosticsSplitPane } from "./diagnostics-split-pane";

describe("DiagnosticsSplitPane", () => {
  beforeEach(() => setWindowWidth(1440));
  afterEach(() => setWindowWidth(0));

  it("sizes a growing pane from its content, so a short column does not clip the list", () => {
    const { container } = render(
      <DiagnosticsSplitPane
        storageKey="test-basis"
        primary={<p>Evidence list</p>}
        secondary={<p>Selected evidence</p>}
        flex={1}
        minHeight={288}
      />,
    );

    const pane = screen.getByText("Evidence list").parentElement?.parentElement as HTMLElement;
    expect(container).toContainElement(pane);
    expect(getComputedStyle(pane).flexBasis).toBe("0%");
    expect(getComputedStyle(pane).flexGrow).toBe("1");
  });

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
    expect(screen.queryByText("Selected evidence")).not.toBeVisible();

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
