import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { Badge, LoomarrProvider, ProgressTrack, StatusDot, Text } from "../index";

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
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <StatusDot label="" tone="ok" />
      </LoomarrProvider>,
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
    expect(markup).toContain("border-top-color:var(--borderControl)");
    expect(markup).toContain("color:var(--contentPrimary)");
    expect(markup).toMatch(/padding-top:1px/);
    expect(markup).not.toMatch(/_(col|backgroundColor|btc)-actionPrim/);
  });
});

describe("Text", () => {
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
    expect(strong).toMatch(/class="[^"]*_fw-600/);
    expect(strong).not.toMatch(/class="[^"]*_fw-500/);
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
    expect(markup.slice(markup.lastIndexOf("<span"))).toMatch(/class="[^"]*\b_tt-uppercase\b/);
  });

  // Amber means focus or the primary action (#1909 Q4): a number role is foreground at rest, and a
  // display numeral asks for amber by name.
  it.each(["channelNumber", "code", "cardChannelNumber", "guideNumber"] as const)(
    "draws %s in foreground unless a display numeral opts in",
    (textRole) => {
      const element = (tone?: "signal") => {
        const markup = renderToStaticMarkup(
          <LoomarrProvider>
            <Text textRole={textRole} tone={tone}>
              07
            </Text>
          </LoomarrProvider>,
        );
        return markup.slice(markup.lastIndexOf("<span"));
      };
      expect(element()).toMatch(/class="[^"]*\b_col-contentPrim/);
      expect(element("signal")).toMatch(/class="[^"]*\b_col-actionPrima/);
    },
  );
});
