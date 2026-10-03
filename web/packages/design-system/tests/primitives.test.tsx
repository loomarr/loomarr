import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { Badge, LoomarrProvider, ProgressTrack, StatusDot } from "../index";

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
});
