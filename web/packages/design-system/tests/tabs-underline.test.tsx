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

  it("accepts a count only where it is drawn, the underline appearance", () => {
    const counted = [{ count: "9", label: "Guide", value: "guide" }] as const;
    render(
      <LoomarrProvider>
        {/* @ts-expect-error The default appearance draws no count, so it takes none. */}
        <Tabs label="Viewer sections" onValueChange={vi.fn()} options={counted} value="guide" />
      </LoomarrProvider>,
    );
    expect(screen.getByRole("tab").textContent).toBe("Guide");
  });

  it("names a tab by its spoken count in place of the bare number", () => {
    render(
      <LoomarrProvider>
        <Tabs
          appearance="underline"
          label="Library sections"
          onValueChange={vi.fn()}
          options={[
            { count: "3,412", countLabel: "3,412 titles", label: "All", value: "all" },
            { count: "2,180", label: "Movies", value: "movies" },
          ]}
          value="all"
        />
      </LoomarrProvider>,
    );
    expect(screen.getByRole("tab", { name: "All, 3,412 titles" })).toBeTruthy();
    // The count is still drawn beside the label.
    expect(screen.getByRole("tab", { name: "All, 3,412 titles" }).textContent).toContain("3,412");
    expect(screen.getByRole("tab", { name: /Movies/ }).textContent).toContain("2,180");
  });

  it("lifts an unselected label to the foreground on hover", async () => {
    render(<Sections />);
    // A label's tone is its atomic classes: a hovered label takes the selected one's, leaving restores.
    const labelOf = (name: RegExp) => screen.getByRole("tab", { name }).querySelector("span")?.className;
    const movies = screen.getByRole("tab", { name: /Movies/ });
    const resting = labelOf(/Movies/);
    expect(resting).not.toBe(labelOf(/All/));
    await userEvent.hover(movies);
    expect(labelOf(/Movies/)).toBe(labelOf(/All/));
    await userEvent.unhover(movies);
    expect(labelOf(/Movies/)).toBe(resting);
  });

  it("scrolls sideways instead of wrapping when the tabs do not fit", () => {
    render(<Sections />);
    const list = screen.getByRole("tablist");
    // The list sits in a horizontal scroller and lays its tabs on one row.
    const scroller = list.parentElement?.parentElement;
    expect(scroller && getComputedStyle(scroller).overflowX).toMatch(/auto|scroll/);
  });
});
