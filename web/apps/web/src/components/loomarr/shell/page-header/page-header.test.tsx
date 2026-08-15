import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { PageHeader } from "./page-header";

describe("PageHeader", () => {
  it("renders a single page title with optional explanatory copy", () => {
    const { rerender } = render(<PageHeader title="Dashboard" />);
    expect(screen.getByRole("heading", { level: 1, name: "Dashboard" })).toBeInTheDocument();
    expect(screen.queryByText("Machine status at a glance.")).not.toBeInTheDocument();

    rerender(<PageHeader title="Dashboard" description="Machine status at a glance." />);
    expect(screen.getByText("Machine status at a glance.")).toBeInTheDocument();
  });

  it("keeps page-level actions inside the shared header", () => {
    render(<PageHeader title="Channels" actions={<button type="button">Add a channel</button>} />);
    const header = screen.getByRole("banner");
    expect(header).toHaveAttribute("data-page-header");
    expect(header).toContainElement(screen.getByRole("button", { name: "Add a channel" }));
  });
});
