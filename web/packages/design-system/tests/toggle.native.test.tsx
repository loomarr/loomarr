// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LoomarrProvider, Toggle, type ToggleProps } from "../index";

// The native Toggle (toggle.tsx): one Pressable carrying the checkbox or switch role.

afterEach(cleanup);

const Controlled = (props: Omit<ToggleProps, "checked" | "onCheckedChange">) => {
  const [checked, setChecked] = useState(false);
  return <Toggle {...props} checked={checked} onCheckedChange={setChecked} />;
};

const renderToggle = (ui: React.ReactNode) => render(<LoomarrProvider>{ui}</LoomarrProvider>);

describe("Toggle (native)", () => {
  it.each(["checkbox", "switch"] as const)("flips a %s when its own label is pressed", async (kind) => {
    renderToggle(<Controlled kind={kind} label="Show episode artwork" />);
    const control = screen.getByRole(kind, { name: "Show episode artwork" });

    await userEvent.click(screen.getByText("Show episode artwork"));
    expect(control.getAttribute("aria-checked")).toBe("true");
  });

  it("keeps a hidden label as the accessible name without printing it", () => {
    renderToggle(<Controlled label="Auto-approve requests" labelHidden />);

    expect(screen.getByRole("checkbox", { name: "Auto-approve requests" })).toBeTruthy();
    expect(screen.queryByText("Auto-approve requests")).toBeNull();
  });

  it("cannot be flipped while disabled", async () => {
    const onCheckedChange = vi.fn();
    renderToggle(<Toggle checked={false} disabled label="Evenings" onCheckedChange={onCheckedChange} />);

    await userEvent.click(screen.getByText("Evenings"), { pointerEventsCheck: 0 });
    expect(onCheckedChange).not.toHaveBeenCalled();
    expect(screen.getByRole("checkbox", { name: "Evenings" }).getAttribute("aria-disabled")).toBe("true");
  });
});
