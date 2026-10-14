import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Label } from "./label";

describe("Label", () => {
  it("associates with its control via htmlFor", () => {
    render(
      <>
        <Label htmlFor="intent">Intent</Label>
        <input id="intent" />
      </>,
    );
    expect(screen.getByText("Intent")).toHaveAttribute("for", "intent");
  });
});
