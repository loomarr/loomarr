// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it, vi } from "vitest";

import { ChannelSeasonalFields, type ChannelSeasonalFieldsProps } from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

type Node = { click(): void; getAttribute(name: string): string | null; textContent?: string | null };
type QueryRoot = { querySelectorAll(selector: string): ArrayLike<Node> };
const testDocument = (
  globalThis as unknown as {
    document: {
      body: { append(node: unknown): void };
      createElement(
        tagName: string,
      ): Parameters<typeof createRoot>[0] & QueryRoot & { remove(): void; textContent?: string | null };
    };
  }
).document;

const HOLIDAYS = [
  { id: "christmas", label: "Christmas" },
  { id: "halloween", label: "Halloween" },
];

const baseProps = (over: Partial<ChannelSeasonalFieldsProps> = {}): ChannelSeasonalFieldsProps => ({
  allHolidaysIncluded: true,
  favourEvenings: true,
  holidays: HOLIDAYS,
  includedHolidayIds: ["christmas", "halloween"],
  mode: "default",
  offSeason: "default",
  onFavourEveningsChange: vi.fn(),
  onHolidayToggle: vi.fn(),
  onModeChange: vi.fn(),
  onOffSeasonChange: vi.fn(),
  onStrengthChange: vi.fn(),
  strength: "default",
  ...over,
});

const mount = async (props: ChannelSeasonalFieldsProps) => {
  const container = testDocument.createElement("div");
  testDocument.body.append(container);
  const root = createRoot(container);
  await act(async () => {
    root.render(
      <LoomarrProvider theme="dark">
        <ChannelSeasonalFields {...props} />
      </LoomarrProvider>,
    );
  });
  return {
    container,
    byRole: (role: string, name: string) => {
      const found = Array.from(container.querySelectorAll(`[role="${role}"]`)).find(
        (node) => node.getAttribute("aria-label") === name,
      );
      return found;
    },
    unmount: async () => {
      await act(async () => root.unmount());
      container.remove();
    },
  };
};

describe("ChannelSeasonalFields", () => {
  it("shows strength and evenings only for the built-in rotation", async () => {
    const view = await mount(baseProps());
    expect(view.byRole("radiogroup", "Holiday strength")).toBeDefined();
    expect(view.byRole("checkbox", "Favour evenings")).toBeDefined();
    expect(view.byRole("radiogroup", "Out of season")).toBeUndefined();
    await view.unmount();

    for (const mode of ["exclusive", "off"] as const) {
      const other = await mount(baseProps({ mode }));
      expect(other.byRole("radiogroup", "Holiday strength")).toBeUndefined();
      expect(other.byRole("checkbox", "Favour evenings")).toBeUndefined();
      await other.unmount();
    }
  });

  it("hides the holiday list when holidays are ignored and shows the fallback for a holiday channel", async () => {
    const off = await mount(baseProps({ mode: "off" }));
    expect(off.byRole("checkbox", "Christmas")).toBeUndefined();
    await off.unmount();

    const exclusive = await mount(baseProps({ mode: "exclusive" }));
    expect(exclusive.byRole("radiogroup", "Out of season")).toBeDefined();
    expect(exclusive.byRole("checkbox", "Christmas")).toBeDefined();
    await exclusive.unmount();
  });

  it("reports each choice it is given", async () => {
    const props = baseProps();
    const view = await mount(props);
    const click = async (role: string, name: string) => {
      const node = view.byRole(role, name);
      if (!node) throw new Error(`missing ${role}: ${name}`);
      await act(async () => node.click());
    };

    await click("radio", "Heavy");
    await click("radio", "Holiday-only channel");
    await click("checkbox", "Halloween");
    await click("checkbox", "Favour evenings");

    expect(props.onStrengthChange).toHaveBeenCalledWith("heavy");
    expect(props.onModeChange).toHaveBeenCalledWith("exclusive");
    expect(props.onHolidayToggle).toHaveBeenCalledWith("halloween", false);
    expect(props.onFavourEveningsChange).toHaveBeenCalledWith(false);
    await view.unmount();
  });

  it("states the consequence of the chosen strength and what evenings does", async () => {
    const view = await mount(
      baseProps({ strengthConsequence: "Halloween is on now. 9 Halloween episodes found: 3 a day." }),
    );
    expect(view.container.textContent).toContain("9 Halloween episodes found: 3 a day.");
    expect(view.container.textContent).toContain("between 5 and 11 pm");
    await view.unmount();

    const anytime = await mount(baseProps({ favourEvenings: false }));
    expect(anytime.container.textContent).toContain("spread across the whole day");
    await anytime.unmount();
  });

  it("says when every built-in holiday is included", async () => {
    const all = await mount(baseProps());
    expect(all.container.textContent).toContain("All built-in holidays are included.");
    await all.unmount();

    const subset = await mount(baseProps({ allHolidaysIncluded: false, includedHolidayIds: ["christmas"] }));
    expect(subset.container.textContent).not.toContain("All built-in holidays are included.");
    await subset.unmount();
  });
});
