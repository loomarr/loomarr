import { createElement, type ReactNode } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { FoundationsStoryShell } from ".";

vi.mock("@loomarr/design-system", () => ({
  LoomarrProvider: ({ children }: { children: ReactNode }) =>
    createElement("section", { "data-theme": "dark" }, children),
  semanticColors: {
    content: { primary: "#F7F8FA" },
    surface: { canvas: "#0B0C0E" },
  },
}));

describe("FoundationsStoryShell", () => {
  it("renders story content on the full-height dark foundation canvas", () => {
    const markup = renderToStaticMarkup(
      <FoundationsStoryShell>
        <span>Foundation content</span>
      </FoundationsStoryShell>,
    );

    expect(markup).toContain("Foundation content");
    expect(markup).toContain("min-height:100vh");
    expect(markup).toContain("background:#0B0C0E");
  });
});
