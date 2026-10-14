import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { PolicyFieldSource } from "./policy-field-source";

describe("PolicyFieldSource", () => {
  it("badges an override and resets it, naming the field", async () => {
    const onReset = vi.fn();
    render(<PolicyFieldSource overridden onReset={onReset} label="Play order" />);
    expect(screen.getByText("Channel override")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Reset to default (Play order)" }));
    expect(onReset).toHaveBeenCalledOnce();
  });

  it("keeps Reset in place but disabled on a default", () => {
    render(<PolicyFieldSource overridden={false} onReset={vi.fn()} label="Play order" />);
    expect(screen.getByText("Default")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Reset to default (Play order)" })).toBeDisabled();
  });

  it("can drop Reset on a default and use its own wording", () => {
    render(
      <PolicyFieldSource
        overridden={false}
        onReset={vi.fn()}
        label="Add new titles without asking"
        overrideLabel="Opted in"
        defaultLabel="Default: off"
        hideResetWhenDefault
      />,
    );
    expect(screen.getByText("Default: off")).toBeInTheDocument();
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });
});
