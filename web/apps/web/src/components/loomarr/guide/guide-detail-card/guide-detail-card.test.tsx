import type { GuideAiring, ImageDTO } from "@loomarr/api";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { GuideDetailCard } from "./guide-detail-card";

const START = Date.UTC(2026, 6, 25, 21, 0, 0);
const STOP = Date.UTC(2026, 6, 25, 21, 30, 0);

const base: GuideAiring = {
  kind: "program",
  scheduleBlockId: "block_heat",
  title: "Heat",
  startMs: START,
  stopMs: STOP,
};
const previewImage: ImageDTO = {
  hash: "preview-art",
  role: "backdrop",
  width: 320,
  height: 180,
  placeholder: "",
  dominantHex: "#111111",
  animated: false,
  srcSetAvif: "",
  srcSetWebp: "/preview.webp 320w",
  src: "/preview.jpg",
};

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

  it("renders the household schedule time instead of the viewer device time", () => {
    render(<GuideDetailCard airing={base} timezone="America/New_York" />);

    expect(screen.getByText("5:00 PM–5:30 PM")).toBeInTheDocument();
  });

  it.each([
    ["programme", { ...base, thumbImage: previewImage }, { width: 80, height: 45 }],
    [
      "filler",
      {
        kind: "filler" as const,
        scheduleBlockId: "block_preview_filler",
        title: "Commercials",
        startMs: START,
        stopMs: STOP,
        thumbImage: { ...previewImage, hash: "filler-hover", role: "thumb", height: 240, animated: true },
        pod: { matchLevel: "exact" as const, totalMs: 30_000, entries: [] },
      },
      { width: 74.667, height: 56 },
    ],
  ])("shows the %s airing's complete preview image", (_kind, airing, expectedSize) => {
    render(<GuideDetailCard airing={airing} />);
    const preview = screen.getByTestId("guide-detail-preview");
    const frame = screen.getByTestId("guide-detail-preview-frame");
    const image = screen.getByTestId("guide-detail-card").querySelector("img");
    // Artwork is context for the title, not a full-width hero that makes the hover card
    // taller than the available viewport on lower Guide rows.
    expect(preview).toHaveClass("h-14", "w-20");
    expect(preview).not.toHaveClass("bg-black");
    expect(preview).not.toHaveClass("border");
    expect(frame).toHaveClass("rounded", "overflow-hidden");
    expect(frame).not.toHaveClass("bg-black", "border");
    expect(Number.parseFloat(frame.style.width)).toBeCloseTo(expectedSize.width, 2);
    expect(Number.parseFloat(frame.style.height)).toBeCloseTo(expectedSize.height, 2);
    expect(image).toHaveAttribute("src", "/preview.jpg");
    // The ThumbHash background now covers exactly the source-shaped frame, never the empty
    // space around a poster/backdrop inside the 80×56 allocation.
    expect(image).toHaveClass("size-full", "object-contain");
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
          scheduleBlockId: "block_break_exact",
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
          scheduleBlockId: "block_break_widened",
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
          scheduleBlockId: "block_break_quiet",
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
          scheduleBlockId: "block_pending",
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
