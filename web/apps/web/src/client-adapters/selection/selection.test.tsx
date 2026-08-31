import { fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

import { BrowserTabs } from "../index";

vi.mock("@loomarr/design-system", () => ({
  Hint: ({ children }: { children: ReactNode }) => children,
  MenuList: () => null,
  SelectControl: () => null,
  Tabs: ({
    label,
    onValueChange,
    options,
    value,
  }: {
    label: string;
    onValueChange: (value: string) => void;
    options: readonly { disabled?: boolean; label: string; value: string }[];
    value: string;
  }) => (
    <div aria-label={label} role="tablist">
      {options.map((option) => (
        <button
          aria-disabled={option.disabled}
          aria-selected={option.value === value}
          disabled={option.disabled}
          key={option.value}
          onClick={() => onValueChange(option.value)}
          role="tab"
          type="button"
        >
          {option.label}
        </button>
      ))}
    </div>
  ),
}));

describe("browser selection adapter", () => {
  it("moves focus past disabled tabs and activates the next available value", () => {
    const onValueChange = vi.fn();
    render(
      <BrowserTabs
        label="Viewer sections"
        onValueChange={onValueChange}
        options={[
          { label: "Watching", value: "watching" },
          { disabled: true, label: "Guide", value: "guide" },
          { label: "Surf", value: "surf" },
        ]}
        value="watching"
      />,
    );

    const watching = screen.getByRole("tab", { name: "Watching" });
    const surf = screen.getByRole("tab", { name: "Surf" });
    watching.focus();
    fireEvent.keyDown(watching, { key: "ArrowRight" });

    expect(surf).toHaveFocus();
    expect(onValueChange).toHaveBeenCalledWith("surf");
  });
});
