import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import {
  ActivityIndicator,
  activityIndicatorMotion,
  LoomarrProvider,
  SignalLoader,
  Skeleton,
  signalLoaderMotion,
  skeletonMotion,
} from "../index";

describe("loading system", () => {
  it("keeps the branded acquisition rhythm stable", () => {
    expect(signalLoaderMotion).toEqual({ bars: 9, duration: 1600, stagger: 90 });
    expect(activityIndicatorMotion.duration).toBe(900);
    expect(skeletonMotion.opacity.high).toBeGreaterThan(skeletonMotion.opacity.low);
  });

  it("renders an accessible static state when motion is reduced", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider theme="dark">
        <SignalLoader detail="Connecting to channel" reducedMotion />
        <ActivityIndicator accessibilityLabel="Saving channel" reducedMotion />
        <Skeleton reducedMotion width="60%" />
      </LoomarrProvider>,
    );

    expect(markup).toContain("TUNING IN");
    expect(markup).toContain("Connecting to channel");
    expect(markup).toContain("Saving channel");
    expect(markup.match(/aria-hidden="true"/g)?.length).toBeGreaterThanOrEqual(10);
  });
});
