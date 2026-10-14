import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { WizardShell } from "./wizard-shell";
import type { WizardShellProps } from "./wizard-shell.type";

const steps = [
  { id: "bootstrap", title: "Admin" },
  { id: "checklist", title: "Connections" },
  { id: "users", title: "Users", optional: true },
];

const renderShell = (props: Partial<WizardShellProps> = {}) =>
  render(
    <WizardShell
      steps={steps}
      currentId="checklist"
      statusById={{ bootstrap: "done", checklist: "current" }}
      title="Connect your services"
      {...props}
    >
      <p>step body</p>
    </WizardShell>,
  );

describe("WizardShell", () => {
  it("renders the step rail and marks the current step", () => {
    renderShell();
    const current = screen.getByRole("listitem", { current: "step" });
    expect(current).toHaveTextContent("Connections");
    expect(screen.getByRole("heading", { name: "Connect your services" })).toBeInTheDocument();
    expect(screen.getByText("step body")).toBeInTheDocument();
  });

  it("shows a skipped step as neutral, not a failure", () => {
    renderShell({ statusById: { bootstrap: "skipped", checklist: "current" } });
    expect(screen.getByText("skipped")).toBeInTheDocument();
  });

  it("drives the nav callbacks", async () => {
    const onBack = vi.fn();
    const onNext = vi.fn();
    const onSkip = vi.fn();
    renderShell({ onBack, onNext, onSkip });

    await userEvent.click(screen.getByRole("button", { name: "Back" }));
    await userEvent.click(screen.getByRole("button", { name: /skip for now/i }));
    await userEvent.click(screen.getByRole("button", { name: "Continue" }));
    expect(onBack).toHaveBeenCalled();
    expect(onSkip).toHaveBeenCalled();
    expect(onNext).toHaveBeenCalled();
  });

  it("blocks advancing while busy or explicitly disabled", () => {
    const { rerender } = renderShell({ onNext: vi.fn(), busy: true });
    expect(screen.getByRole("button", { name: /continue/i })).toBeDisabled();

    rerender(
      <WizardShell
        steps={steps}
        currentId="checklist"
        statusById={{}}
        title="t"
        onNext={vi.fn()}
        nextDisabled
      >
        <p>step body</p>
      </WizardShell>,
    );
    expect(screen.getByRole("button", { name: /continue/i })).toBeDisabled();
  });
});
