import type { IncomingPipelineDTO } from "@loomarr/api";
import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ClipPipeline } from "./clip-pipeline";

// The pipeline strip is the only thing telling an operator that forty downloaded clips are being
// worked on rather than stuck. Every assertion here is a way it could say something untrue.
const LADDER = ["probe", "transcode", "split", "language", "transcribe", "tag", "vision", "score"];

const at = (over: Partial<IncomingPipelineDTO> = {}): IncomingPipelineDTO => ({
  hash: "hash-cola",
  name: "Coca-Cola 1985",
  stage: "tag",
  status: "running",
  // -1 is the "this rung cannot measure itself" sentinel, and it is the DEFAULT here on purpose:
  // most rungs cannot measure themselves, so a fixture defaulting to a real percentage would make
  // the measured case look like the ordinary one.
  progress: -1,
  stages: [
    { stage: "probe", status: "done", at: "2026-08-08T10:00:00Z" },
    { stage: "transcode", status: "done", at: "2026-08-08T10:00:20Z" },
    {
      stage: "split",
      status: "skipped",
      note: "it is a single advert, not a compilation",
      at: "2026-08-08T10:00:21Z",
    },
    { stage: "language", status: "done", at: "2026-08-08T10:00:40Z" },
    {
      stage: "transcribe",
      status: "skipped",
      note: "the description already says enough",
      at: "2026-08-08T10:00:41Z",
    },
  ],
  updatedAt: "2026-08-08T10:01:00Z",
  ...over,
});

describe("ClipPipeline — strip", () => {
  // ⚠ THE rule the whole component is shaped around. `row.stages` is the VISITED ladder, so a
  // strip built from it would have five pips here and eight at the end — an operator could never
  // see how far there is left to go, and the bar would appear to grow rather than fill.
  it("draws one pip per LADDER rung, not per visited record", () => {
    render(<ClipPipeline row={at()} ladder={LADDER} />);

    expect(screen.getAllByRole("listitem")).toHaveLength(LADDER.length);
  });

  it("names every rung and its state, because colour is never the only signal", () => {
    render(<ClipPipeline row={at()} ladder={LADDER} />);

    const list = screen.getByRole("list", { name: "Progress for Coca-Cola 1985" });
    expect(within(list).getByText("Check the file: done")).toBeInTheDocument();
    expect(within(list).getByText("Find the ads inside: skipped")).toBeInTheDocument();
    expect(within(list).getByText("Work out what it is: in progress")).toBeInTheDocument();
    // Rungs the clip has not reached are "not started", NOT absent and NOT "done".
    expect(within(list).getByText("Score it: not started")).toBeInTheDocument();
  });

  // The runner records a rung only once it RESOLVES, so the stage mid-run has no visited record.
  // Reading state from `row.stages` alone would draw the rung actually being worked on as if
  // nothing had started — the queue would look stalled at exactly the moment it is busiest.
  it("reads the current rung from the row's own position, not from the visited ladder", () => {
    render(<ClipPipeline row={at({ stage: "vision", status: "running" })} ladder={LADDER} />);

    expect(screen.getByText("Look at the picture: in progress")).toBeInTheDocument();
  });
});

describe("ClipPipeline — list", () => {
  it("puts the skip REASON inline, so a stage that did not happen does not read as broken", () => {
    render(<ClipPipeline row={at()} ladder={LADDER} variant="list" />);

    expect(screen.getByText(/the description already says enough/)).toBeInTheDocument();
  });

  it("uses the active voice for the rung being worked on", () => {
    render(<ClipPipeline row={at()} ladder={LADDER} variant="list" />);

    expect(screen.getByText("Working out what it is")).toBeInTheDocument();
    // …and the plain label for one that has not started.
    expect(screen.getByText("Score it")).toBeInTheDocument();
  });

  // ⚠ A 0-width bar reads as "no progress" rather than "no measurement" — a different and false
  // claim. Only transcode can measure itself; Whisper and an LLM turn are single opaque calls.
  it("renders NO bar for a running rung that cannot measure itself", () => {
    render(<ClipPipeline row={at({ progress: -1 })} ladder={LADDER} variant="list" />);

    expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
  });

  it("renders a bar only for the rung that measured one", () => {
    render(<ClipPipeline row={at({ progress: 62, stage: "transcode" })} ladder={LADDER} variant="list" />);

    const bar = screen.getByRole("progressbar");
    expect(bar).toHaveAttribute("aria-valuenow", "62");
  });

  // Scoped to the failed rung, not the list: one clip failing must announce, forty rungs quietly
  // succeeding must not.
  it("announces a failure on the rung that failed", () => {
    render(<ClipPipeline row={at({ stage: "vision", status: "failed" })} ladder={LADDER} variant="list" />);

    expect(within(screen.getByRole("alert")).getByText("Look at the picture")).toBeInTheDocument();
  });

  // ⚠ A newer backend adding a rung must not blank it out of a ladder that claims to be complete.
  // The raw id tells an operator — and a bug report — more than "Unknown stage" would.
  it("falls back to the server's own id for a stage this build has no copy for", () => {
    render(<ClipPipeline row={at()} ladder={[...LADDER, "fingerprint"]} variant="list" />);

    expect(screen.getByText("fingerprint")).toBeInTheDocument();
  });
});
