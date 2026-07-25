import type { GuideAiring } from "@loomarr/api";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { GuideDetailCard } from "./guide-detail-card";

const START = Date.UTC(2026, 6, 25, 21, 0, 0);
const STOP = Date.UTC(2026, 6, 25, 21, 30, 0);

const base: GuideAiring = { kind: "program", title: "Heat", startMs: START, stopMs: STOP };

describe("GuideDetailCard", () => {
  // Rendering nothing for a null subject lets the caller mount this unconditionally instead of
  // guarding at every use site.
  it("renders nothing without a subject", () => {
    const { container } = render(<GuideDetailCard airing={null} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("shows a programme's identifying facts", () => {
    render(
      <GuideDetailCard
        airing={{
          ...base,
          year: 1995,
          rating: "R",
          genres: ["Action", "Crime"],
          runtimeMs: 170 * 60_000,
          description: "A crew of professional thieves.",
        }}
      />,
    );
    expect(screen.getByText(/1995/)).toBeInTheDocument();
    expect(screen.getByText(/2h 50m/)).toBeInTheDocument();
    expect(screen.getByText(/professional thieves/)).toBeInTheDocument();
  });

  // An episode needs BOTH names and its numbering, or the card says less than the block did.
  it("shows the series and episode numbering for an episode", () => {
    render(
      <GuideDetailCard
        airing={{ ...base, title: "Bart the Mother", series: "The Simpsons", season: 10, episode: 3 }}
      />,
    );
    expect(screen.getByText("The Simpsons")).toBeInTheDocument();
    expect(screen.getByText(/S10E03/)).toBeInTheDocument();
  });

  // THE BREAK CASE — the one that could not exist before V13b, because the API had only a
  // channel-wide pool and nothing could say what plays in THIS break.
  it("lists the clips that play in a break, with era and quality", () => {
    render(
      <GuideDetailCard
        airing={{
          kind: "filler",
          title: "Break",
          startMs: START,
          stopMs: STOP,
          pod: {
            matchLevel: "exact",
            totalMs: 65000,
            entries: [
              { name: "Channel bumper", kind: "bumper", durationMs: 5000, isFallbackCard: false },
              {
                name: "Sunny D",
                kind: "commercial",
                durationMs: 30000,
                era: 1994,
                quality: "480p",
                isFallbackCard: false,
              },
            ],
          },
        }}
      />,
    );
    expect(screen.getByText("Sunny D")).toBeInTheDocument();
    expect(screen.getByText("1994")).toBeInTheDocument();
    // Quality explains a grainy advert as an authentic capture rather than a playback fault.
    expect(screen.getByText("480p")).toBeInTheDocument();
  });

  // "Why are my commercials wrong" is the question a break prompts, so the ladder's verdict is
  // stated in words — not left as a level name the viewer must decode.
  it("explains how far down the fallback ladder assembly went", () => {
    render(
      <GuideDetailCard
        airing={{
          kind: "filler",
          title: "Break",
          startMs: START,
          stopMs: STOP,
          pod: {
            matchLevel: "widened",
            totalMs: 30000,
            entries: [{ name: "Some ad", kind: "commercial", durationMs: 30000, isFallbackCard: false }],
          },
        }}
      />,
    );
    expect(screen.getByText(/Era widened/)).toBeInTheDocument();
    expect(screen.getByText(/widened the era/i)).toBeInTheDocument();
  });

  // An exact match is the QUIET case: a chip saying "everything is fine" is noise on every
  // well-matched break, which is most of them.
  it("stays quiet when the match was exact", () => {
    render(
      <GuideDetailCard
        airing={{
          kind: "filler",
          title: "Break",
          startMs: START,
          stopMs: STOP,
          pod: {
            matchLevel: "exact",
            totalMs: 30000,
            entries: [{ name: "Some ad", kind: "commercial", durationMs: 30000, isFallbackCard: false }],
          },
        }}
      />,
    );
    expect(screen.queryByText(/widened/i)).not.toBeInTheDocument();
  });

  // Provenance is what turns a pending slot from a mystery into a status.
  it("shows provenance for a pending slot", () => {
    render(
      <GuideDetailCard
        airing={{
          kind: "pending",
          title: "Dune: Part Two",
          startMs: START,
          stopMs: STOP,
          nominal: true,
          provenance: "requested · 41h left",
        }}
      />,
    );
    expect(screen.getByText(/Pending slot/)).toBeInTheDocument();
    expect(screen.getByText(/41h left/)).toBeInTheDocument();
  });
});
