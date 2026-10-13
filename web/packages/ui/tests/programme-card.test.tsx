import { LoomarrProvider } from "@loomarr/design-system";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { ProgrammeCard, type ProgrammeCardData } from "../index";

const programme: ProgrammeCardData = {
  artworkState: "ready",
  channelLogoState: "missing",
  channelName: "Late Night Sci-Fi",
  channelNumber: "07",
  progressPercent: 57,
  seriesTitle: "A sci-fi anthology",
  timeLabel: "24 min left",
  title: "The pilot",
};

const render = (element: React.ReactElement) =>
  renderToStaticMarkup(<LoomarrProvider>{element}</LoomarrProvider>);

// One card model, three layouts (#1659): each draws the same data, and missing artwork is the
// frame's own state in all of them.
describe("ProgrammeCard layouts", () => {
  it("stacks the On now card: still, channel, programme, time left", () => {
    const markup = render(<ProgrammeCard layout="stacked" programme={programme} />);
    const order = ["07", "Late Night Sci-Fi", "A sci-fi anthology · The pilot", "24 min left"].map((text) =>
      markup.indexOf(text),
    );
    expect(order.every((at) => at >= 0)).toBe(true);
    expect([...order].sort((a, b) => a - b)).toEqual(order);
  });

  it("lays the Watching now card over the still, with who is watching", () => {
    const markup = render(
      <ProgrammeCard
        layout="overlay"
        programme={programme}
        viewer={{ device: "living room TV", initials: "MA", name: "A household member" }}
      />,
    );
    expect(markup).toContain("MA");
    expect(markup).toContain("A household member");
    expect(markup).toContain("· living room TV");
    expect(markup).toContain("A sci-fi anthology · The pilot");
    expect(markup).toContain("linear-gradient");
  });

  it("draws nothing about a viewer it was not given", () => {
    const markup = render(<ProgrammeCard layout="overlay" programme={programme} />);
    expect(markup).not.toContain("living room TV");
    expect(markup).toContain("Late Night Sci-Fi");
  });

  it("shows the frame's missing-artwork state in every layout", () => {
    for (const layout of ["default", "stacked", "overlay"] as const) {
      const markup = render(
        <ProgrammeCard
          artwork={<span>still</span>}
          layout={layout}
          programme={{ ...programme, artworkState: "missing" }}
        />,
      );
      expect(markup, layout).toContain("No artwork");
      expect(markup, layout).not.toContain("still</span>");
    }
  });

  // The native card (TV, phone) is the default and must not move.
  it("keeps the default layout's identity rows", () => {
    const markup = render(<ProgrammeCard programme={programme} />);
    expect(markup).toContain("LN");
    // No artwork scrim here; the hatch behind the monogram (#1659 decision N8) is unrelated.
    expect(markup).not.toContain("linear-gradient(180deg");
  });
});
