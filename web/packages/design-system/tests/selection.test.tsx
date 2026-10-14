import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { Hint, LoomarrProvider, MenuList, SelectControl, Tabs } from "../index";

describe("shared selection and disclosure controls", () => {
  it("publishes one selected tab and keeps unavailable tabs out of traversal", () => {
    const output = renderToStaticMarkup(
      <LoomarrProvider>
        <Tabs
          label="Viewer sections"
          onValueChange={vi.fn()}
          options={[
            { label: "Guide", value: "guide" },
            { disabled: true, label: "Recordings", value: "recordings" },
          ]}
          value="guide"
        />
      </LoomarrProvider>,
    );
    expect(output).toContain('role="tablist"');
    expect(output).toContain('aria-selected="true"');
    expect(output).toContain('aria-disabled="true"');
  });

  it("renders collapsed selection semantics and an expanded single-choice list", () => {
    const output = renderToStaticMarkup(
      <LoomarrProvider>
        <SelectControl
          label="Theme"
          onOpenChange={vi.fn()}
          onValueChange={vi.fn()}
          open
          options={[
            { label: "Dark", value: "dark" },
            { label: "Light", value: "light" },
          ]}
          value="dark"
        />
      </LoomarrProvider>,
    );
    expect(output).toContain('role="button"');
    expect(output).toContain('aria-expanded="true"');
    expect(output).toContain('role="radiogroup"');
  });

  it("keeps menus and hints presentational while adapters own placement and triggers", () => {
    const output = renderToStaticMarkup(
      <LoomarrProvider>
        <MenuList
          items={[{ label: "Disconnect", tone: "danger", value: "disconnect" }]}
          label="Device actions"
          onSelect={vi.fn()}
        />
        <Hint content="Returns to live playback." visible>
          <span>Go live</span>
        </Hint>
      </LoomarrProvider>,
    );
    expect(output).toContain('role="menu"');
    expect(output).toContain('role="menuitem"');
    expect(output).toContain('role="tooltip"');
    expect(output).toContain("Returns to live playback.");
  });
});
