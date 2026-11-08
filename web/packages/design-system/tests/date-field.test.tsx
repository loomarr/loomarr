// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { DateField, LoomarrProvider } from "../index";

// The web DateField is the browser's own date input in the Field's box. What it owes a browser user
// is pinned by driving it: its printed label names and focuses it, the browser's value reaches the
// caller, and the help line is read with it. The native adapter is a Field and is not exercised here.

afterEach(cleanup);

const renderField = (ui: React.ReactNode) => render(<LoomarrProvider>{ui}</LoomarrProvider>);

describe("DateField (web)", () => {
  it("is a date input its label names and focuses", async () => {
    renderField(<DateField label="Air date" onValueChange={vi.fn()} value="" />);

    await userEvent.click(screen.getByText("Air date"));

    const input = screen.getByLabelText("Air date");
    expect(input.getAttribute("type")).toBe("date");
    expect(document.activeElement).toBe(input);
  });

  it("hands the browser's value to the caller, and a local date and time for datetime", () => {
    const onValueChange = vi.fn();
    renderField(<DateField kind="datetime" label="At" onValueChange={onValueChange} value="" />);
    const input = screen.getByLabelText("At");

    fireEvent.change(input, { target: { value: "2026-11-08T21:30" } });

    expect(input.getAttribute("type")).toBe("datetime-local");
    expect(onValueChange).toHaveBeenCalledWith("2026-11-08T21:30");
  });

  it("is read with its help line and cannot be changed while disabled", () => {
    renderField(
      <DateField
        description="When it first aired."
        disabled
        label="Air date"
        onValueChange={vi.fn()}
        value=""
      />,
    );
    const input = screen.getByLabelText<HTMLInputElement>("Air date");
    const describedBy = input.getAttribute("aria-describedby") ?? "";

    expect(document.getElementById(describedBy)?.textContent).toBe("When it first aired.");
    expect(input.disabled).toBe(true);
  });
});
