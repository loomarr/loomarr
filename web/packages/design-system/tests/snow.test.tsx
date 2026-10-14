import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { AnalogSnow, LoomarrProvider, SignalLoader, Text } from "../index";

const render = (node: React.ReactNode) =>
  renderToStaticMarkup(<LoomarrProvider theme="dark">{node}</LoomarrProvider>);

// The parts the TV channel-switch readout needs (#1627): React Native can't draw the web's
// filtered snow, can't track text, takes one text shadow, and SignalLoader always named itself.
describe("B4 readout primitives", () => {
  it("draws the snow from a seeded tile, the same frame every time, inert", () => {
    const fills = (markup: string) => [...markup.matchAll(/rgb\(\d+,\d+,\d+\)/g)].map((m) => m[0]).join();
    const once = render(<AnalogSnow reducedMotion />);
    expect(fills(once)).toBe(fills(render(<AnalogSnow reducedMotion />)));
    expect(fills(once)).not.toBe(fills(render(<AnalogSnow reducedMotion seed={9} />)));
    expect(once).not.toContain("feTurbulence");
    expect(once).toContain('aria-hidden="true"');
    // The grey ramp, faintly cold: every cell's blue is at least its red.
    const cells = [...once.matchAll(/rgb\((\d+),(\d+),(\d+)\)/g)].map((m) => m.slice(1).map(Number));
    expect(cells.length).toBeGreaterThan(500);
    expect(cells.every(([r, g, b]) => r === g && (b ?? 0) >= (r ?? 0))).toBe(true);
  });

  it("tracks text in em of its own size, and haloes it with a hidden wide copy", () => {
    const tracked = render(
      <Text density="tv" halo textRole="metadata" tracking={0.24}>
        TUNING IN
      </Text>,
    );
    // metadata is 16 at TV density: 0.24em is 3.84.
    expect(tracked).toContain("letter-spacing:3.84px");
    expect(tracked.match(/TUNING IN/g)).toHaveLength(2);
    expect(tracked).toContain('aria-hidden="true"');
    expect(render(<Text textRole="metadata">plain</Text>).match(/plain/g)).toHaveLength(1);
  });

  it("draws SignalLoader's bars alone when the caller sets its own readout", () => {
    const bars = render(<SignalLoader accessibilityLabel="Tuning in" label={null} reducedMotion />);
    expect(bars).not.toContain("TUNING IN");
    expect(bars).toContain('aria-label="Tuning in"');
    expect(render(<SignalLoader reducedMotion />)).toContain("TUNING IN");
  });
});
