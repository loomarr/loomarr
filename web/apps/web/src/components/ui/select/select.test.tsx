import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "./select";

// The app's enum control (a themed listbox over Base UI Select).
const Window = ({ onChange = () => {} }: { onChange?: (v: unknown) => void }) => (
  <Select defaultValue="240" onValueChange={onChange}>
    <SelectTrigger aria-label="Window span">
      <SelectValue />
    </SelectTrigger>
    <SelectContent>
      <SelectItem value="120">2 hours</SelectItem>
      <SelectItem value="240">4 hours</SelectItem>
      <SelectItem value="360">6 hours</SelectItem>
    </SelectContent>
  </Select>
);

describe("Select", () => {
  // ⚠ THE POINT OF THIS TEST. Base UI resolves `<SelectValue>` to the selected item's LABEL only
  // when `<Select.Root>` is given an `items` prop; without one it can fall back to the raw value.
  // Every Select in this app passes its options as inline `<SelectItem>` JSX and no `items`, so if
  // that fallback applied, each trigger would read "240" where the list says "4 hours" — a
  // user-visible regression across 13 call sites that no type error and no axe rule would catch.
  it("shows the selected item's label on the trigger, not its raw value", () => {
    render(<Window />);

    const trigger = screen.getByRole("combobox", { name: "Window span" });
    expect(trigger).toHaveTextContent("4 hours");
    expect(trigger).not.toHaveTextContent("240");
  });

  it("stays closed until the trigger is activated", async () => {
    render(<Window />);
    expect(screen.queryByRole("option", { name: "2 hours" })).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("combobox", { name: "Window span" }));
    expect(await screen.findByRole("option", { name: "2 hours" })).toBeInTheDocument();
  });

  it("marks the current value as the selected option", async () => {
    render(<Window />);
    await userEvent.click(screen.getByRole("combobox", { name: "Window span" }));

    expect(await screen.findByRole("option", { name: "4 hours" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("option", { name: "2 hours" })).toHaveAttribute("aria-selected", "false");
  });

  it("reports the chosen value through onValueChange", async () => {
    const onChange = vi.fn();
    render(<Window onChange={onChange} />);

    await userEvent.click(screen.getByRole("combobox", { name: "Window span" }));
    await userEvent.click(await screen.findByRole("option", { name: "6 hours" }));

    expect(onChange).toHaveBeenCalled();
    expect(onChange.mock.calls[0]?.[0]).toBe("360");
  });
});
