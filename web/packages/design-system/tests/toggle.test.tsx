// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LoomarrProvider, Toggle, type ToggleProps } from "../index";

// The web Toggle, on Base UI. What it owes a browser user is pinned by driving it: its own label
// and a caller's <label htmlFor> both flip it, Space flips it from the keyboard, and a disabled one
// cannot be flipped. The native adapter is a Pressable and is not exercised here.

afterEach(cleanup);

const Controlled = (props: Omit<ToggleProps, "checked" | "onCheckedChange">) => {
  const [checked, setChecked] = useState(false);
  return <Toggle {...props} checked={checked} onCheckedChange={setChecked} />;
};

const renderToggle = (ui: React.ReactNode) => render(<LoomarrProvider>{ui}</LoomarrProvider>);

describe("Toggle (web)", () => {
  it.each(["checkbox", "switch"] as const)("flips a %s when its own label is clicked", async (kind) => {
    renderToggle(<Controlled kind={kind} label="Show episode artwork" />);
    const control = screen.getByRole(kind, { name: "Show episode artwork" });

    await userEvent.click(screen.getByText("Show episode artwork"));
    expect(control.getAttribute("aria-checked")).toBe("true");
  });

  it("flips from a caller's own <label htmlFor> when its label is hidden", async () => {
    renderToggle(
      <>
        <label htmlFor="auto-approve">Auto-approve requests</label>
        <Controlled id="auto-approve" label="Auto-approve requests" labelHidden />
      </>,
    );

    await userEvent.click(screen.getByText("Auto-approve requests"));
    expect(screen.getByRole("checkbox", { name: "Auto-approve requests" }).getAttribute("aria-checked")).toBe(
      "true",
    );
  });

  it("flips with Space from the keyboard", async () => {
    renderToggle(<Controlled kind="switch" label="Follow system theme" />);
    await userEvent.tab();
    await userEvent.keyboard(" ");
    expect(screen.getByRole("switch", { name: "Follow system theme" }).getAttribute("aria-checked")).toBe(
      "true",
    );
  });

  it("cannot be flipped while disabled", async () => {
    const onCheckedChange = vi.fn();
    renderToggle(<Toggle checked={false} disabled label="Evenings" onCheckedChange={onCheckedChange} />);

    await userEvent.click(screen.getByText("Evenings"));
    expect(onCheckedChange).not.toHaveBeenCalled();
    expect(screen.getByRole("checkbox", { name: "Evenings" }).getAttribute("aria-disabled")).toBe("true");
  });
});
