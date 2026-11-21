// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { LoomarrProvider, Tabs } from "../index";

// The underline appearance changes the look only: the roles, names and keyboard are today's Tabs'.

afterEach(cleanup);

type Section = "all" | "movies" | "shows" | "collections";
const OPTIONS = [
  { count: "3,412", label: "All", value: "all" },
  { count: "2,180", label: "Movies", value: "movies" },
  { count: "1,232", label: "Shows", value: "shows" },
  { disabled: true, label: "Collections", value: "collections" },
] as const;

const Sections = ({ onChange = vi.fn() }: { onChange?: (value: Section) => void }) => {
  const [value, setValue] = useState<Section>("all");
  return (
    <LoomarrProvider>
      <Tabs
        appearance="underline"
        label="Library sections"
        onValueChange={(next) => {
          setValue(next);
          onChange(next);
        }}
        options={OPTIONS}
        value={value}
      />
    </LoomarrProvider>
  );
};

describe("Tabs, underline appearance", () => {
  it("is a named tablist of tabs, each named by its label and count", () => {
    render(<Sections />);
    expect(screen.getByRole("tablist", { name: "Library sections" })).toBeTruthy();
    const tabs = screen.getAllByRole("tab");
    expect(tabs).toHaveLength(4);
    expect(tabs[0]?.textContent).toContain("All");
    expect(tabs[0]?.textContent).toContain("3,412");
    expect(tabs[3]?.textContent).toBe("Collections");
  });

  it("marks one tab selected and keeps only that tab in the tab order", () => {
    render(<Sections />);
    const [all, movies, , collections] = screen.getAllByRole("tab");
    expect(all?.getAttribute("aria-selected")).toBe("true");
    expect(movies?.getAttribute("aria-selected")).toBe("false");
    expect(all?.getAttribute("tabindex")).toBe("0");
    expect(movies?.getAttribute("tabindex")).toBe("-1");
    expect(collections?.getAttribute("aria-disabled")).toBe("true");
    expect(collections?.getAttribute("tabindex")).toBe("-1");
  });

  it("selects a tab on press and moves the tab stop to it", async () => {
    const onChange = vi.fn();
    render(<Sections onChange={onChange} />);
    await userEvent.click(screen.getByRole("tab", { name: /Movies/ }));
    expect(onChange).toHaveBeenCalledWith("movies");
    expect(screen.getByRole("tab", { name: /Movies/ }).getAttribute("aria-selected")).toBe("true");
    expect(screen.getByRole("tab", { name: /Movies/ }).getAttribute("tabindex")).toBe("0");
    expect(screen.getByRole("tab", { name: /All/ }).getAttribute("tabindex")).toBe("-1");
  });

  // Today's Tabs answer Enter (react-native-web presses a non-button role on Enter only), and so
  // does this appearance.
  it("reaches the selected tab with Tab and activates a focused tab with Enter", async () => {
    const onChange = vi.fn();
    render(<Sections onChange={onChange} />);
    await userEvent.tab();
    expect(document.activeElement).toBe(screen.getByRole("tab", { name: /All/ }));
    screen.getByRole("tab", { name: /Shows/ }).focus();
    await userEvent.keyboard("{Enter}");
    expect(onChange).toHaveBeenLastCalledWith("shows");
    expect(screen.getByRole("tab", { name: /Shows/ }).getAttribute("aria-selected")).toBe("true");
  });

  it("never selects a disabled tab", async () => {
    const onChange = vi.fn();
    render(<Sections onChange={onChange} />);
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    await user.click(screen.getByRole("tab", { name: "Collections" }));
    expect(onChange).not.toHaveBeenCalled();
  });

  it("leaves the default appearance as outlined actions", () => {
    render(
      <LoomarrProvider>
        <Tabs
          label="Viewer sections"
          onValueChange={vi.fn()}
          options={[{ count: "9", label: "Guide", value: "guide" }]}
          value="guide"
        />
      </LoomarrProvider>,
    );
    // The default draws no count: it is the underline appearance's alone.
    expect(screen.getByRole("tab").textContent).toBe("Guide");
  });
});
