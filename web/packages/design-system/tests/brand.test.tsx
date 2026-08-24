import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { BrandLockup, BrandMark, brandChroma, brandLaunchMotion, LoomarrProvider } from "../index";

describe("Loomarr brand", () => {
  it("renders the canonical seven-segment chroma bar in order", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <BrandMark contained={false} />
      </LoomarrProvider>,
    );
    const renderedSegments = [...markup.matchAll(/<rect fill="(#[A-F0-9]{6})"/g)].map((match) => match[1]);

    expect(brandChroma).toEqual([
      "#FFB020",
      "#F5D90A",
      "#3DD68C",
      "#4CC9E8",
      "#D6409F",
      "#E5484D",
      "#8B93A3",
    ]);
    expect(renderedSegments).toEqual([...brandChroma]);
  });

  it("keeps the supplied power-on sequence timing", () => {
    expect(brandLaunchMotion).toEqual({
      segmentDuration: 340,
      segmentStagger: 40,
      wordDuration: 400,
      wordDelay: 220,
      taglineDelay: 320,
    });
  });

  it("leaves document heading semantics to the screen that owns the lockup", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <BrandLockup />
      </LoomarrProvider>,
    );

    expect(markup).toContain("LOOMARR");
    expect(markup).not.toContain('role="heading"');
  });
});
