import { LoomarrProvider } from "@loomarr/design-system";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { TuneBars, TuneStatic } from "../index";

const render = (node: React.ReactNode, theme: "dark" | "light" = "dark") =>
  renderToStaticMarkup(<LoomarrProvider theme={theme}>{node}</LoomarrProvider>);

// The snow is the only thing that draws an SVG.
const hasStatic = (markup: string) => markup.includes("<svg");

describe("TuneStatic", () => {
  it("draws static over the picture while it covers, and none once clear", () => {
    for (const cover of ["burst", "fading", "wash", "cold"] as const) {
      const markup = render(<TuneStatic cover={cover} reducedMotion={false} />);
      expect(hasStatic(markup), cover).toBe(true);
      expect(markup).toContain('aria-hidden="true"');
    }
    expect(hasStatic(render(<TuneStatic cover="none" reducedMotion={false} />))).toBe(false);
  });

  it("draws no static under reduced motion, only the darkened picture to crossfade", () => {
    for (const cover of ["burst", "wash", "cold"] as const) {
      const markup = render(<TuneStatic cover={cover} reducedMotion />);
      expect(hasStatic(markup), cover).toBe(false);
      expect(markup).toContain(`tune-static-${cover}`);
    }
  });

  it("keeps the same dim snow in the light theme", () => {
    const snow = <TuneStatic cover="wash" reducedMotion={false} />;
    const veil = (markup: string) => markup.slice(markup.indexOf("<svg"));
    expect(veil(render(snow, "light"))).toBe(veil(render(snow)));
  });
});

describe("TuneBars", () => {
  const colours = (markup: string) =>
    [...markup.matchAll(/background-color:rgba?\((\d+),(\d+),(\d+)/g)].map((m) => m.slice(1, 4).join(","));
  const chroma = [
    "255,176,32",
    "245,217,10",
    "61,214,140",
    "76,201,232",
    "214,64,159",
    "229,72,77",
    "139,147,163",
  ];

  it("waits in two greys and locks to the seven colours", () => {
    const waiting = colours(render(<TuneBars bars="waiting" flip={1} reducedMotion={false} />));
    expect(waiting).toHaveLength(7);
    expect(new Set(waiting)).toEqual(new Set(["139,147,163", "90,97,112"]));
    expect(colours(render(<TuneBars bars="locked" flip={1} reducedMotion={false} />))).toEqual(chroma);
  });

  it("runs the seven colours on a flip, and does not run them under reduced motion", () => {
    expect(colours(render(<TuneBars bars="run" flip={1} reducedMotion={false} />))).toEqual(chroma);
    expect(render(<TuneBars bars="run" flip={1} reducedMotion />)).not.toContain("tune-bars");
    expect(render(<TuneBars bars="waiting" flip={1} reducedMotion />)).toContain("tune-bars-waiting");
    expect(render(<TuneBars bars="none" flip={1} reducedMotion={false} />)).not.toContain("tune-bars");
  });
});
