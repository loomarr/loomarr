import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
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

  it("renders as the supplied element when `render` is set", () => {
    render(<Button render={<a href="/channels" />}>Go</Button>);

    const link = screen.getByRole("link", { name: "Go" });
    expect(link).toHaveAttribute("href", "/channels");
    // The button's own styling still applies — `render` composes, it does not replace.
    expect(link).toHaveClass("inline-flex");
  });

  // `mergeProps` chains handlers and concatenates className rather than letting one side win.
  // A plain spread would drop whichever onClick came first, silently.
  it("keeps both the rendered element's props and the button's own", async () => {
    const onClick = vi.fn();
    render(
      <Button variant="outline" onClick={onClick} render={<a href="/channels" className="custom" />}>
        Go
      </Button>,
    );

    const link = screen.getByRole("link", { name: "Go" });
    expect(link).toHaveClass("custom");
    expect(link).toHaveClass("border-input");
    await userEvent.click(link);
    expect(onClick).toHaveBeenCalledOnce();
  });
});
