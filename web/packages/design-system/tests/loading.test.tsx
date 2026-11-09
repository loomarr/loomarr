import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import {
  ActivityIndicator,
  activityIndicatorMotion,
  brandChroma,
  LoadingStrip,
  LoomarrProvider,
  loadingStripMotion,
  SignalLoader,
  Skeleton,
  signalLoaderMotion,
  skeletonMotion,
} from "../index";
import { withoutRuntimeMarker } from "./support/atomic-css";

describe("loading system", () => {
  it("keeps the branded acquisition rhythm stable", () => {
    expect(signalLoaderMotion).toEqual({ bars: 9, duration: 1600, stagger: 90 });
    expect(activityIndicatorMotion.duration).toBe(900);
    expect(skeletonMotion.opacity.high).toBeGreaterThan(skeletonMotion.opacity.low);
  });

  it("renders an accessible static state when motion is reduced", () => {
    const markup = withoutRuntimeMarker(
      renderToStaticMarkup(
        <LoomarrProvider theme="dark">
          <SignalLoader detail="Connecting to channel" reducedMotion />
          <ActivityIndicator accessibilityLabel="Saving channel" reducedMotion />
          <Skeleton reducedMotion width="60%" />
        </LoomarrProvider>,
      ),
    );

    expect(markup).toContain("TUNING IN");
    expect(markup).toContain("Connecting to channel");
    expect(markup).toContain("Saving channel");
    expect(markup.match(/aria-hidden="true"/g)?.length).toBeGreaterThanOrEqual(10);
  });
});

// The page and section wait (#2789): the brand bars as a thin strip.
describe("LoadingStrip", () => {
  const render = (element: React.ReactElement) =>
    withoutRuntimeMarker(renderToStaticMarkup(<LoomarrProvider theme="dark">{element}</LoomarrProvider>));

  it("names the wait as a progress bar and draws the brand's seven bars", () => {
    const markup = render(<LoadingStrip accessibilityLabel="Loading the guide" reducedMotion />);
    expect(markup).toContain('role="progressbar"');
    expect(markup).toContain('aria-label="Loading the guide"');
    for (const color of brandChroma) expect(markup.toLowerCase()).toContain(hexToRgb(color));
  });

  it("is the still bars under reduced motion: every bar fully lit, nothing moving", () => {
    const markup = render(<LoadingStrip accessibilityLabel="Loading" reducedMotion />);
    expect(markup).not.toMatch(/opacity:\s*0\.35/);
    expect(markup).not.toContain("transform");
  });

  it("stays out of the accessibility tree when its caller names the wait", () => {
    const markup = render(<LoadingStrip decorative reducedMotion />);
    expect(markup).not.toContain('role="progressbar"');
    expect(markup).toContain('aria-hidden="true"');
  });

  it("travels a light across the bars with opacity alone", () => {
    expect(loadingStripMotion.opacity.lit).toBeGreaterThan(loadingStripMotion.opacity.rest);
    expect(Object.keys(loadingStripMotion)).not.toContain("transform");
  });
});

const hexToRgb = (hex: string) => {
  const n = Number.parseInt(hex.slice(1), 16);
  return `rgba(${(n >> 16) & 255},${(n >> 8) & 255},${n & 255},1.00)`;
};
