// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ChoiceGroup, LoomarrProvider } from "../index";

afterEach(cleanup);

const options = [
  { label: "2h", value: "2" },
  { description: "Longer than most people look ahead", label: "12h", value: "12" },
  { disabled: true, label: "24h", value: "24" },
] as const;

const draw = (onValueChange = vi.fn()) => {
  render(
    <LoomarrProvider theme="dark">
      <ChoiceGroup label="Span" onValueChange={onValueChange} options={options} value="2" />
    </LoomarrProvider>,
  );
  return onValueChange;
};

describe("ChoiceGroup", () => {
  it("is a named radio group with the chosen option checked", () => {
    draw();

    expect(screen.getByRole("radiogroup", { name: "Span" })).toBeTruthy();
    expect(screen.getByRole("radio", { name: "2h" }).getAttribute("aria-checked")).toBe("true");
    expect(screen.getByRole("radio", { name: "12h" }).getAttribute("aria-checked")).toBe("false");
  });

  it("hands back the value of the option that is pressed", async () => {
    const onValueChange = draw();

    await userEvent.click(screen.getByRole("radio", { name: "12h" }));

    expect(onValueChange).toHaveBeenCalledWith("12");
  });

  it("draws a disabled option as disabled and out of reach of a press", () => {
    const onValueChange = draw();

    const disabled = screen.getByRole("radio", { name: "24h" });

    expect(disabled.getAttribute("aria-disabled")).toBe("true");
    expect(getComputedStyle(disabled).pointerEvents).toBe("none");
    expect(onValueChange).not.toHaveBeenCalled();
  });

  it("shows an option's description beneath its label", () => {
    draw();
    expect(screen.getByText("Longer than most people look ahead")).toBeTruthy();
  });
});
