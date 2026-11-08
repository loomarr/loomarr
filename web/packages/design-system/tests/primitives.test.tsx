import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { Badge, LoomarrProvider, ProgressTrack, StatusDot, Text } from "../index";
import { declarationsOf, stylesOfText, tagBefore, withoutRuntimeMarker } from "./support/atomic-css";

describe("StatusDot", () => {
  it("carries the tone's colour and an accessible name", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <StatusDot label="Failed" tone="error" />
      </LoomarrProvider>,
    );
    expect(markup).toContain('role="img"');
    expect(markup).toContain('aria-label="Failed"');
  });

  it("hides an unlabelled dot from assistive tech", () => {
    const markup = withoutRuntimeMarker(
      renderToStaticMarkup(
        <LoomarrProvider>
          <StatusDot label="" tone="ok" />
        </LoomarrProvider>,
      ),
    );
    expect(markup).toContain("aria-hidden");
    expect(markup).not.toContain('role="img"');
  });

  it("renders a compact size at 6px", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <StatusDot label="" size="compact" tone="off" />
      </LoomarrProvider>,
    );
    expect(markup).toMatch(/width:6px/);
  });
});

describe("ProgressTrack", () => {
  // react-native-web flattens the fill's style to inline rgba().
  const fill = (markup: string) => markup.match(/background-color:(rgba\([^)]*\));[^"]*width:40%/)?.[1];

  it("fills elapsed airing time in foreground by default, never amber (#1909 Q3)", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <ProgressTrack percent={40} />
      </LoomarrProvider>,
    );
    expect(fill(markup)).toBe("rgba(231,234,240,1.00)");
  });

  it("fills a download with tune", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <ProgressTrack percent={40} tone="tune" />
      </LoomarrProvider>,
    );
    expect(fill(markup)).toBe("rgba(76,201,232,1.00)");
  });

  it("stays decorative with no label, as every current caller expects", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <ProgressTrack percent={40} />
      </LoomarrProvider>,
    );
    expect(markup).not.toContain('role="progressbar"');
  });

  it("exposes the aria-value trio once a label is given", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <ProgressTrack label="Clip preparation" percent={40} />
      </LoomarrProvider>,
    );
    expect(markup).toContain('role="progressbar"');
    expect(markup).toContain('aria-valuenow="40"');
    expect(markup).toContain('aria-valuemin="0"');
    expect(markup).toContain('aria-valuemax="100"');
  });

  it("omits aria-valuenow when indeterminate", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <ProgressTrack label="Working" />
      </LoomarrProvider>,
    );
    expect(markup).toContain('role="progressbar"');
    expect(markup).not.toContain("aria-valuenow");
  });
});

describe("Badge", () => {
  it("renders the legacy square shape's brand-accent tones without a border", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <Badge shape="square" tone="suggest">
          AI
        </Badge>
      </LoomarrProvider>,
    );
    expect(markup).toContain(">AI<");
    expect(markup).not.toMatch(/border-?[wW]idth:1px/);
  });

  // A block-level flex box here would stretch the tag across any block parent and pin it to the top
  // of an `items-center` row; a colour on the label alone would miss the icon and hover classes.
  it("sits inline on web and hands its colour down from the wrapper", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <Badge icon={<svg aria-hidden="true" />} shape="square" tone="override">
          Set
        </Badge>
      </LoomarrProvider>,
    );
    expect(markup).toMatch(/<span class="is_View[^>]*style="color:var\(--contentPrimary\)"><svg/);
    expect(markup).toContain("display:inline-flex");
    expect(markup).not.toContain("<div");
  });

  // The base suggest stop fails AA on its own 15% tint; the -300 stop and the neutral text pass.
  it("draws square labels in their AA-safe text stops", () => {
    const label = (tone: "neutral" | "suggest") =>
      renderToStaticMarkup(
        <LoomarrProvider>
          <Badge shape="square" tone={tone}>
            Tag
          </Badge>
        </LoomarrProvider>,
      ).toUpperCase();
    expect(label("suggest")).toContain("COLOR:#DC5BAC");
    expect(label("neutral")).toContain("COLOR:#8B93A3");
  });

  // Caps are reserved for channel numbers, clocks and provenance (#2778): a status or tag chip
  // reads in sentence case, as written.
  it("keeps a square tag's label in sentence case", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <Badge shape="square">Needs attention</Badge>
      </LoomarrProvider>,
    );
    expect(markup).toContain(">Needs attention<");
    // Tamagui emits the transform as an atomic `_tt-` class on the label's own span.
    expect(markup).not.toMatch(/<span class="is_Text[^"]* _tt-\d+/);
  });

  it("carries a tooltip on the tag itself", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <Badge shape="square" title="Clips directly assigned this tag">
          3 direct
        </Badge>
      </LoomarrProvider>,
    );
    expect(markup).toContain('title="Clips directly assigned this tag"');
  });

  it("lowers the accent tint on request", () => {
    const tinted = (tintOpacity?: number) =>
      renderToStaticMarkup(
        <LoomarrProvider>
          <Badge shape="square" tintOpacity={tintOpacity} tone="onair">
            Admin
          </Badge>
        </LoomarrProvider>,
      );
    expect(tinted()).toContain(",0.15)");
    expect(tinted(0.12)).toContain(",0.12)");
  });

  it("keeps the existing pill shape as the default", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <Badge tone="success">Signal locked</Badge>
      </LoomarrProvider>,
    );
    expect(markup).toContain(">Signal locked<");
  });

  // An override is neutral (#1909 Q5): a foreground label and a control hairline, no amber, and
  // the hairline comes out of the padding so it is as tall as Default.
  it("draws the square override tone neutral with a hairline at Default's height", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <Badge shape="square" tone="override">
          Channel override
        </Badge>
      </LoomarrProvider>,
    );
    expect(markup).toContain("border-color:var(--borderControl)");
    expect(markup).toContain("color:var(--contentPrimary)");
    expect(markup).toMatch(/padding-block:1px/);
    expect(markup).not.toContain("var(--actionPrimary)");
  });
});

describe("Text", () => {
  it("announces a live region on the web as aria-live, not as an unknown DOM prop", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <Text accessibilityLiveRegion="polite" textRole="metadata">
          2m behind
        </Text>
        <Text accessibilityLiveRegion="none" textRole="metadata">
          quiet
        </Text>
      </LoomarrProvider>,
    );
    expect(markup).toContain('aria-live="polite"');
    expect(markup).toContain('aria-live="off"');
    expect(markup.toLowerCase()).not.toContain("accessibilityliveregion");
  });

  it("sets a strong label at 600 without changing the role's size", () => {
    const strong = renderToStaticMarkup(
      <LoomarrProvider>
        <Text strong textRole="cardLabel">
          Chosen
        </Text>
        <Text as="span" strong textRole="cardLabel">
          Chosen
        </Text>
      </LoomarrProvider>,
    );
    // Tamagui's own Text takes an atomic class; the `as` host takes inline style.
    const tamaguiStyles = declarationsOf(strong, /<span class="is_Text[^>]*>/.exec(strong)?.[0] ?? "");
    expect(tamaguiStyles).toContain("font-weight:600");
    expect(tamaguiStyles).not.toContain("font-weight:500");
    expect(strong).toMatch(/<span[^>]*font-weight:600/);
    expect(strong).toMatch(/<span[^>]*font-size:13px/);
  });

  it("renders a given host element on web and resolves the colour inline", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <Text as="p" textRole="caption">
          hi
        </Text>
      </LoomarrProvider>,
    );
    expect(markup).toContain("<p");
    expect(markup).toContain("color:#E7EAF0");
  });

  it("shouts: uppercase with wide tracking", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <Text as="span" shout textRole="caption">
          hi
        </Text>
      </LoomarrProvider>,
    );
    expect(markup.slice(markup.lastIndexOf("<span"))).toContain("text-transform:uppercase");
  });

  it("keeps a caller's own textTransform when not shouting", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <Text textRole="guideLabel" textTransform="uppercase">
          hi
        </Text>
      </LoomarrProvider>,
    );
    // The element's own atomic class, not the provider's injected stylesheet — that sheet
    // carries `text-transform:uppercase` for other components whether or not this Text has it.
    expect(declarationsOf(markup, tagBefore(markup, "hi"))).toContain("text-transform:uppercase");
  });

  // Amber means focus or the primary action (#1909 Q4): a number role is foreground at rest, and a
  // display numeral asks for amber by name.
  it.each(["channelNumber", "code", "cardChannelNumber", "guideNumber"] as const)(
    "draws %s in foreground unless a display numeral opts in",
    (textRole) => {
      const colour = (tone?: "signal") => {
        const markup = renderToStaticMarkup(
          <LoomarrProvider>
            <Text textRole={textRole} tone={tone}>
              07
            </Text>
          </LoomarrProvider>,
        );
        return /(?:^|;)color:([^;]+)/.exec(stylesOfText(markup, "07<"))?.[1];
      };
      expect(colour()).toBe("var(--contentPrimary)");
      expect(colour("signal")).toBe("var(--actionPrimary)");
    },
  );
});
