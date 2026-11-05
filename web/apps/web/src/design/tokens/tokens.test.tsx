import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Tokens } from "./tokens";

// The index's one promise is that every generated token appears with its value. A nested group
// (a text style, a motion curve) once rendered as "[object Object]", which is a token on the page
// whose value you cannot read.
describe("Tokens", () => {
  it("shows every token as a readable leaf value", () => {
    render(<Tokens />);
    expect(screen.queryByText("[object Object]")).toBeNull();
    const rows = screen.getAllByRole("row");
    expect(rows.length).toBeGreaterThan(0);
    for (const row of rows) {
      const value = row.querySelectorAll("code")[1]?.textContent ?? "";
      expect(value, `row ${row.textContent}`).not.toBe("");
      expect(value).not.toContain("[object");
    }
  });

  it("keys a nested token by its dotted path", () => {
    render(<Tokens />);
    const keys = screen.getAllByRole("row").map((row) => row.querySelector("code")?.textContent ?? "");
    expect(keys.some((key) => key.includes("."))).toBe(true);
  });
});
