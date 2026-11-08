// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { CompactSelect, type CompactSelectProps, LoomarrProvider } from "../index";

// The web CompactSelect is a native <select>. What it owes a browser user is pinned by driving it:
// choosing changes the value, a row's own label can name it, and its help and error wiring reach the
// control. The native adapter is the field select and is not exercised here.

afterEach(cleanup);

const ranges = [
  { label: "Last hour", value: "1h" },
  { label: "Last 6 hours", value: "6h" },
  { disabled: true, label: "Last week", value: "7d" },
] as const;

type Range = (typeof ranges)[number]["value"];

const Controlled = (props: Partial<CompactSelectProps<Range>>) => {
  const [value, setValue] = useState<Range>("1h");
  return (
    <CompactSelect label="Time range" onValueChange={setValue} options={ranges} value={value} {...props} />
  );
};

const renderSelect = (ui: React.ReactNode) => render(<LoomarrProvider>{ui}</LoomarrProvider>);

describe("CompactSelect (web)", () => {
  it("is named by its label and changes to the chosen option", async () => {
    renderSelect(<Controlled />);
    const select = screen.getByRole<HTMLSelectElement>("combobox", { name: "Time range" });

    await userEvent.selectOptions(select, "Last 6 hours");
    expect(select.value).toBe("6h");
    expect(screen.getByRole<HTMLOptionElement>("option", { name: "Last week" }).disabled).toBe(true);
  });

  it("takes its name from a row's own label and reads that row's help with it", () => {
    renderSelect(
      <>
        <span id="range-label">Keep logs for</span>
        <span id="range-help">Older lines are dropped.</span>
        <Controlled aria-describedby="range-help" aria-labelledby="range-label" invalid />
      </>,
    );
    const select = screen.getByRole("combobox", {
      name: "Keep logs for",
      description: "Older lines are dropped.",
    });

    expect(select.getAttribute("aria-invalid")).toBe("true");
  });

  it("cannot be changed while disabled", () => {
    renderSelect(<Controlled disabled />);
    expect(screen.getByRole<HTMLSelectElement>("combobox", { name: "Time range" }).disabled).toBe(true);
  });
});
