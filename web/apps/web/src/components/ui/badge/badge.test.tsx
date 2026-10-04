import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Badge } from "./badge";

describe("Badge", () => {
  // Sourced from @loomarr/design-system's shared tokens (#970 PR B): `suggest`'s text colour is
  // `semanticColors.accent.suggest`, the same AA-safe -300 stop the legacy `text-suggest-300`
  // utility resolved to.
  it("renders its label and applies the variant's AA-safe stop", () => {
    render(<Badge variant="suggest">AI</Badge>);
    const el = screen.getByText("AI");
    expect(el).toBeInTheDocument();
    expect(el).toHaveStyle({ color: "#DC5BAC" });
  });

  it("defaults to the neutral variant", () => {
    render(<Badge>Bumper</Badge>);
    expect(screen.getByText("Bumper")).toHaveStyle({ color: "#8B93A3" });
  });
});
