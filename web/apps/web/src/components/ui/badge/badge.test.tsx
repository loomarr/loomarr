import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Badge } from "./badge";

describe("Badge", () => {
  it("renders its label and applies the variant's AA-safe stop", () => {
    render(<Badge variant="suggest">AI</Badge>);
    const el = screen.getByText("AI");
    expect(el).toBeInTheDocument();
    expect(el.className).toContain("text-suggest-300");
  });

  it("defaults to the neutral variant", () => {
    render(<Badge>Bumper</Badge>);
    expect(screen.getByText("Bumper").className).toContain("text-static-400");
  });
});
