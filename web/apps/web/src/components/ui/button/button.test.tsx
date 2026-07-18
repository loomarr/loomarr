import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Button } from "./button";

describe("Button", () => {
  it("renders its label as a button", () => {
    render(<Button>Approve</Button>);
    expect(screen.getByRole("button", { name: "Approve" })).toBeInTheDocument();
  });

  it("honors the disabled attribute", () => {
    render(<Button disabled>Nope</Button>);
    expect(screen.getByRole("button", { name: "Nope" })).toBeDisabled();
  });

  it("renders as a child element when asChild is set", () => {
    render(
      <Button asChild>
        <a href="/channels">Go</a>
      </Button>,
    );
    expect(screen.getByRole("link", { name: "Go" })).toHaveAttribute("href", "/channels");
  });
});
