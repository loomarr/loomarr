import type { ProposalJourneyProgressDTO } from "@loomarr/api/models/proposalJourneyProgressDTO";
import { act, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { progressLine } from "@/components/loomarr/feedback/progress-line";
import { GenerationProgress } from "./generation-progress";

const NOW = new Date("2026-09-25T12:00:12Z");

const progress = (over: Partial<ProposalJourneyProgressDTO> = {}): ProposalJourneyProgressDTO => ({
  stage: "choosing",
  terms: ["speed"],
  picks: [],
  target: 8,
  startedAt: "2026-09-25T12:00:00Z",
  ...over,
});

const speed = {
  key: "movie:tmdb:100",
  mediaType: "movie",
  name: "Speed",
  year: 1994,
  tmdbId: 100,
  inLibrary: false,
} as const;
const matrix = {
  key: "movie:tmdb:603",
  mediaType: "movie",
  name: "The Matrix",
  year: 1999,
  tmdbId: 603,
  inLibrary: true,
} as const;

afterEach(() => vi.useRealTimers());

describe("GenerationProgress", () => {
  it.each([
    ["reasoning", "Understanding your channel…"],
    ["searching", "Looking for matching titles…"],
    ["scoring", "Preparing your channel…"],
  ] as const)("shows one calm status for %s before the first snapshot", (phase, label) => {
    const { container } = render(<GenerationProgress phase={phase} round={4} elapsedSeconds={3} />);
    expect(screen.getByText(label)).toBeInTheDocument();
    expect(container.querySelectorAll(".animate-spin")).toHaveLength(1);
    expect(screen.queryByText(/pass|\d+s/i)).not.toBeInTheDocument();
  });

  it("adds reassuring copy for a longer wait", () => {
    render(<GenerationProgress phase="reasoning" elapsedSeconds={12} />);
    expect(screen.getByText(/can take a little while/i)).toBeInTheDocument();
  });

  it("leaves a terminal failure to the recovery surface", () => {
    const { container } = render(<GenerationProgress phase="failed" error="Generation failed" />);
    expect(container).toBeEmptyDOMElement();
  });

  const stages: [ProposalJourneyProgressDTO["stage"], string[], string][] = [
    ["reading", [], "Reading your request…"],
    ["searching", ["speed", "action"], "Searching your library for speed, action…"],
    ["searching", [], "Searching your library…"],
    ["choosing", ["speed"], "Choosing titles…"],
    ["building", ["speed"], "Building your lineup…"],
  ];
  it.each(stages)("names the real stage %s (terms %j): %s", (stage, terms, line) => {
    render(<GenerationProgress phase="reasoning" progress={progress({ stage, terms })} />);
    expect(screen.getByRole("status")).toHaveTextContent(line);
    expect(progressLine(progress({ stage, terms }))).toBe(line);
  });

  it("streams each chosen title as a row, with an honest count and elapsed time", () => {
    vi.useFakeTimers({ now: NOW });
    render(<GenerationProgress phase="reasoning" progress={progress({ picks: [speed, matrix] })} />);
    const list = screen.getByRole("list", { name: "Titles chosen so far" });
    const rows = within(list).getAllByRole("listitem");
    expect(rows).toHaveLength(2);
    expect(rows[0]).toHaveTextContent("Speed");
    expect(rows[0]).toHaveTextContent("1994");
    expect(rows[0]).toHaveTextContent("Will be added");
    expect(rows[1]).toHaveTextContent("The Matrix");
    expect(rows[1]).toHaveTextContent("In your library");
    expect(screen.getByText("2 of about 8 picked · 12 s")).toBeInTheDocument();
  });

  it("counts on from the server's start time, one clock per second", () => {
    vi.useFakeTimers({ now: NOW });
    render(<GenerationProgress phase="reasoning" progress={progress({ picks: [speed] })} />);
    act(() => vi.advanceTimersByTime(3000));
    expect(screen.getByText("1 of about 8 picked · 15 s")).toBeInTheDocument();
  });

  it("shows no count line, and no empty list, before the first pick", () => {
    render(<GenerationProgress phase="reasoning" progress={progress()} />);
    expect(screen.queryByRole("list")).not.toBeInTheDocument();
    expect(screen.queryByText(/picked/)).not.toBeInTheDocument();
  });

  // The live region announces the stage only: a per-second clock or a growing list inside it
  // would make a screen reader chatter through the whole wait.
  it("keeps the ticking clock and the list out of the live region", () => {
    render(<GenerationProgress phase="reasoning" progress={progress({ picks: [speed] })} />);
    const live = document.querySelector("[aria-live]");
    expect(live).toHaveTextContent("Choosing titles…");
    expect(live).not.toHaveTextContent("Speed");
    expect(live).not.toHaveTextContent(/picked|\d s/);
  });

  it("uses one motion, and none for reduced motion", () => {
    const { container } = render(<GenerationProgress phase="reasoning" progress={progress()} />);
    const spinner = container.querySelector(".animate-spin");
    expect(container.querySelectorAll(".animate-spin, .animate-pulse")).toHaveLength(1);
    expect(spinner?.getAttribute("class")).toContain("motion-reduce:animate-none");
  });
});
