import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { LoginShell } from "./login-shell";

describe("LoginShell", () => {
  it("frames its content with the brand hero", () => {
    render(
      <LoginShell>
        <button type="button">Sign in</button>
      </LoginShell>,
    );
    // The identity is present around the slotted content (the actual coverage this closes).
    expect(screen.getByText("LOOMARR")).toBeInTheDocument();
    expect(screen.getByText("always something on")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Sign in" })).toBeInTheDocument();
  });
});
