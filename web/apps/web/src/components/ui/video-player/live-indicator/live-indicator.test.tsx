import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { LiveIndicator } from "./live-indicator";

describe("LiveIndicator", () => {
  it("shows the LIVE label", () => {
    render(<LiveIndicator />);
    expect(screen.getByText("Live")).toBeInTheDocument();
  });
});
