import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { ErrorDetails } from "./error-details";

describe("ErrorDetails", () => {
  // ⚠ Collapsed by default: one failing row in a list must not push every other row down the
  // page. The message is in the DOM (so it is findable/printable), just not shown.
  it("hides the message until asked", async () => {
    render(<ErrorDetails message="no FILLER_DIR configured" />);
    expect(screen.getByText("no FILLER_DIR configured")).not.toBeVisible();

    await userEvent.click(screen.getByText("Show error"));
    expect(screen.getByText("no FILLER_DIR configured")).toBeVisible();
  });

  // ⚠ The empty case renders NOTHING, so a caller can pass an optional field straight through
  // without guarding at the call site. Covered here rather than as a story: the gallery harness
  // waits for an element and a null-rendering story times out (V13 hit exactly that).
  it("renders nothing without a message", () => {
    const { container } = render(<ErrorDetails />);
    expect(container).toBeEmptyDOMElement();
  });

  it("takes custom labels", async () => {
    render(<ErrorDetails message="boom" showLabel="Why did this fail?" hideLabel="Collapse" />);
    await userEvent.click(screen.getByText("Why did this fail?"));
    expect(screen.getByText("Collapse")).toBeInTheDocument();
  });
});
