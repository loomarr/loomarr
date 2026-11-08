import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { render } from "@/test/render";
import { PolicyFieldSource } from "./policy-field-source";

describe("PolicyFieldSource", () => {
  it("badges an override and resets it, naming the field", async () => {
    const onReset = vi.fn();
    render(<PolicyFieldSource overridden onReset={onReset} label="Play order" />);
    expect(screen.getByText("Channel override")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Reset to default (Play order)" }));
    expect(onReset).toHaveBeenCalledOnce();
  });

  // #2744: a Reset beside the Default badge would have nothing to reset.
  it("offers no Reset on a default", () => {
    render(<PolicyFieldSource overridden={false} onReset={vi.fn()} label="Play order" />);
    expect(screen.getByText("Default")).toBeInTheDocument();
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("uses its own wording", () => {
    render(
      <PolicyFieldSource
        overridden={false}
        onReset={vi.fn()}
        label="Add new titles without asking"
        overrideLabel="Opted in"
        defaultLabel="Default: off"
      />,
    );
    expect(screen.getByText("Default: off")).toBeInTheDocument();
  });
});
