import { CHANNEL_IDENT_HUES } from "@loomarr/core/guide";
import { channelIdentColors } from "@loomarr/design-system";
import { compositeTint, contrast, staticScale } from "@loomarr/design-system/web-theme";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ArtworkFallback } from "./artwork-fallback";

describe("ArtworkFallback", () => {
  it("is a tinted wash, never hatched", () => {
    render(<ArtworkFallback label="Night Train" />);
    const el = screen.getByRole("presentation");
    expect(el.style.backgroundImage).toContain("linear-gradient");
    expect(el.style.backgroundImage).not.toContain("repeating");
  });

  it("is never blank for a title with no channel: it sets the title's letters", () => {
    render(<ArtworkFallback label="Night Train" />);
    expect(screen.getByText("NT")).toBeInTheDocument();
  });

  it("shows the channel's monogram, tinted by its identity hue", () => {
    render(<ArtworkFallback channel={{ name: "Field Notes", number: 7 }} />);
    expect(screen.getByText("FN").style.color).toBe("var(--color-signal)");
  });

  // The full pink fails on its own wash; its on-tint stop is the `-300` (#3011).
  it("derives a different hue for a different channel number, lettered in its on-tint stop", () => {
    render(<ArtworkFallback channel={{ name: "Night Owl", number: 1 }} />);
    expect(screen.getByText("NO").style.color).toBe("var(--color-suggest-300)");
  });

  // The letters are large text (text-3xl, semibold), so 3:1, measured where the gradient's wash is
  // strongest. The design system's ident contrast test proves the stops themselves (#3011).
  it("keeps every hue's letters at large-text AA where the wash is strongest", () => {
    for (const hue of CHANNEL_IDENT_HUES) {
      const { unmount } = render(
        <ArtworkFallback channel={{ name: "Night Owl", number: CHANNEL_IDENT_HUES.indexOf(hue) }} />,
      );
      const peak = Number(
        /(\d+)%, var\(--color-static-900\)/.exec(screen.getByRole("presentation").style.backgroundImage)?.[1],
      );
      const colors = channelIdentColors("dark")[hue];
      const ground = compositeTint(colors.wash, peak, staticScale["static-900"]);
      expect(contrast(colors.text, ground), hue).toBeGreaterThanOrEqual(3);
      unmount();
    }
  });

  it("prefers the channel's letters over a label", () => {
    render(<ArtworkFallback channel={{ name: "Night Owl", number: 1 }} label="Other Thing" />);
    expect(screen.queryByText("OT")).not.toBeInTheDocument();
  });

  it("merges a caller className with its own layout classes", () => {
    render(<ArtworkFallback className="rounded-md" />);
    const el = screen.getByRole("presentation");
    expect(el.className).toContain("rounded-md");
    expect(el.className).toContain("size-full");
  });
});
