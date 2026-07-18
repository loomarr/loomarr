import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Input } from "./input";

describe("Input", () => {
  it("forwards value and placeholder", () => {
    render(<Input placeholder="Search…" defaultValue="matrix" aria-label="search" />);
    const el = screen.getByLabelText("search") as HTMLInputElement;
    expect(el).toHaveAttribute("placeholder", "Search…");
    expect(el.value).toBe("matrix");
  });
});
