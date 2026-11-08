import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import {
  Disclosure,
  LoomarrProvider,
  resolveAdaptiveDirection,
  ScrollFrame,
  SectionHeader,
  Text,
} from "../index";

describe("shared adaptive layout", () => {
  it("resolves narrow and wide composition from density-specific breakpoints", () => {
    expect(resolveAdaptiveDirection(899, "pointer")).toBe("column");
    expect(resolveAdaptiveDirection(900, "pointer")).toBe("row");
    expect(resolveAdaptiveDirection(1279, "tv")).toBe("column");
    expect(resolveAdaptiveDirection(1280, "tv")).toBe("row");
    expect(resolveAdaptiveDirection(600, "touch", 580)).toBe("row");
  });

  it("publishes disclosure state and only renders expanded content when requested", () => {
    const collapsed = renderToStaticMarkup(
      <LoomarrProvider>
        <Disclosure expanded={false} label="Episode information" onExpandedChange={vi.fn()}>
          Classic episode details
        </Disclosure>
      </LoomarrProvider>,
    );
    const expanded = renderToStaticMarkup(
      <LoomarrProvider theme="light">
        <Disclosure expanded label="Episode information" onExpandedChange={vi.fn()}>
          Classic episode details
        </Disclosure>
      </LoomarrProvider>,
    );

    expect(collapsed).toContain('aria-expanded="false"');
    expect(collapsed).not.toContain("Classic episode details");
    expect(expanded).toContain('aria-expanded="true"');
    expect(expanded).toContain("Classic episode details");
    const controls = expanded.match(/aria-controls="([^"]+)"/);
    expect(controls).not.toBeNull();
    expect(expanded).toContain(`id="${controls?.[1]}"`);
  });

  it("keeps scroll content behind the package interface", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <ScrollFrame>
          <Text textRole="body">Scrollable programme detail</Text>
        </ScrollFrame>
      </LoomarrProvider>,
    );

    expect(markup).toContain("Scrollable programme detail");
    expect(markup).toContain("flex-grow:1");
  });

  it("titles a section with a labelled heading, its meta and trailing controls", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <SectionHeader id="home-watching" meta="3 people" title="Watching now">
          <a href="/guide">Full guide</a>
        </SectionHeader>
      </LoomarrProvider>,
    );

    expect(markup).toMatch(/<h2[^>]*id="home-watching"[^>]*>Watching now<\/h2>/);
    expect(markup).toContain("3 people");
    expect(markup).toContain('<a href="/guide">Full guide</a>');
  });
});
