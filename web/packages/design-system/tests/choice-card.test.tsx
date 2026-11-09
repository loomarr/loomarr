// @vitest-environment jsdom
import { act } from "react";
import { createRoot } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { ChoiceCard, Icon, icons, LoomarrProvider } from "../index";

const markupOf = (node: React.ReactNode) =>
  renderToStaticMarkup(<LoomarrProvider theme="dark">{node}</LoomarrProvider>);

describe("ChoiceCard", () => {
  it("names the choice with its title and describes it", () => {
    const markup = markupOf(<ChoiceCard description="Cartoons, then a movie" title="Saturday morning" />);
    expect(markup).toContain("Saturday morning");
    expect(markup).toContain("Cartoons, then a movie");
  });

  it("announces the chosen state for both chosen and not", () => {
    expect(markupOf(<ChoiceCard selected title="A" />)).toContain('aria-pressed="true"');
    expect(markupOf(<ChoiceCard selected={false} title="A" />)).toContain('aria-pressed="false"');
    expect(markupOf(<ChoiceCard title="A" />)).not.toContain("aria-pressed");
  });

  it("draws the chosen card differently from the rest", () => {
    expect(markupOf(<ChoiceCard selected title="A" />)).not.toEqual(
      markupOf(<ChoiceCard selected={false} title="A" />),
    );
  });

  it("draws a leading glyph before the words", () => {
    const markup = markupOf(
      <ChoiceCard leading={<Icon decorative glyph={icons.suggest} tone="suggest" />} title="A" />,
    );
    expect(markup.indexOf("<svg")).toBeGreaterThan(-1);
    expect(markup.indexOf("<svg")).toBeLessThan(markup.indexOf(">A<"));
  });

  it("runs onPress, and not when disabled", () => {
    const onPress = vi.fn();
    const container = document.createElement("div");
    document.body.append(container);
    const root = createRoot(container);
    act(() => {
      root.render(
        <LoomarrProvider theme="dark">
          <ChoiceCard onPress={onPress} title="Open" />
          <ChoiceCard disabled onPress={onPress} title="Shut" />
        </LoomarrProvider>,
      );
    });
    const [first, second] = [...container.querySelectorAll<HTMLElement>('[role="button"]')];
    act(() => {
      first?.click();
      second?.click();
    });
    expect(onPress).toHaveBeenCalledTimes(1);
    act(() => root.unmount());
    container.remove();
  });
});
