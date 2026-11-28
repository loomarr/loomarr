// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { LoomarrProvider, TabBar } from "../index";

afterEach(cleanup);

describe("TabBar in a DOM", () => {
  it("reads a badged tab's text as the label, a space, then the number", () => {
    render(
      <LoomarrProvider theme="dark">
        <TabBar
          accessibilityLabel="Primary navigation"
          idiom="ios"
          items={[{ badge: 4, icon: "library", label: "Library", value: "library" }]}
          onSelect={vi.fn()}
          selected="library"
        />
      </LoomarrProvider>,
    );
    // Text nodes joined in order, with nothing added between elements, as axe reads them.
    expect(screen.getByRole("tab").textContent).toBe("Library 4");
  });
});
