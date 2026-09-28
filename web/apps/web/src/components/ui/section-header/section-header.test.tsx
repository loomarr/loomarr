import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { SectionHeader, SectionHeaderAction } from "./section-header";

describe("SectionHeader", () => {
  it("is a level-2 heading a section can point aria-labelledby at", () => {
    render(<SectionHeader id="tonight" title="Tonight" meta="4 highlights" />);
    const heading = screen.getByRole("heading", { level: 2, name: "Tonight" });
    expect(heading).toHaveAttribute("id", "tonight");
    expect(screen.getByText("4 highlights")).toBeInTheDocument();
  });

  // The arrow is decoration. A screen reader announcing "Full guide right arrow" is noise.
  it("names its trailing action without the arrow", () => {
    render(
      <SectionHeader title="Tonight">
        <SectionHeaderAction>Full guide</SectionHeaderAction>
      </SectionHeader>,
    );
    expect(screen.getByRole("button", { name: "Full guide" })).toBeInTheDocument();
  });

  // A control ("Different ideas") leads with its icon and has no "go somewhere" arrow.
  it("drops the arrow when the action carries an icon", () => {
    render(
      <SectionHeader title="Channel ideas">
        <SectionHeaderAction icon={<svg aria-hidden data-testid="icon" />}>
          Different ideas
        </SectionHeaderAction>
      </SectionHeader>,
    );
    const action = screen.getByRole("button", { name: "Different ideas" });
    expect(action).toContainElement(screen.getByTestId("icon"));
    expect(action).not.toHaveTextContent("→");
  });

  // Navigation must be a real link (middle-click, open in new tab), so `render` swaps the element.
  it("renders the action as whatever element it is given", () => {
    render(
      <SectionHeader title="On the way">
        <SectionHeaderAction render={<a href="/requests" />}>All requests</SectionHeaderAction>
      </SectionHeader>,
    );
    expect(screen.getByRole("link", { name: "All requests" })).toHaveAttribute("href", "/requests");
  });
});
