import type { IncomingPipelineDTO } from "@loomarr/api";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { PipelineSection, sentenceFor } from "./pipeline-section";

const LADDER = ["probe", "transcode", "split", "language", "transcribe", "tag", "vision", "score"];

const row = (over: Partial<IncomingPipelineDTO> = {}): IncomingPipelineDTO => ({
  hash: "hash-cola",
  name: "Coca-Cola 1985",
  stage: "tag",
  status: "running",
  progress: -1,
  stages: [{ stage: "probe", status: "done", at: "2026-08-08T10:00:00Z" }],
  updatedAt: "2026-08-08T10:01:00Z",
  ...over,
});

// `sentenceFor` is what the operator actually reads on a collapsed row, so its wording is tested
// directly rather than only through the rendered section — each branch is a different claim about
// what the machine is doing.
describe("sentenceFor", () => {
  it("uses the active voice for the rung being worked on", () => {
    expect(sentenceFor(row())).toBe("Working out what it is");
  });

  it("says a queued clip is waiting rather than working", () => {
    expect(sentenceFor(row({ stage: "vision", status: "queued" }))).toBe("Waiting to look at the picture");
  });

  // ⚠ "retrying" is the honest word: a failed rung is not terminal for the clip — the runner
  // retries it, and the pipeline row only rejects once the attempts are spent.
  it("says a failed rung is being retried", () => {
    expect(sentenceFor(row({ stage: "transcode", status: "failed" }))).toBe(
      "Level the sound — failed, retrying",
    );
  });

  // A rung this build has no copy for comes from a newer backend; the raw id beats a placeholder.
  it("falls back to the server's id for an unknown stage", () => {
    expect(sentenceFor(row({ stage: "fingerprint" }))).toBe("fingerprint");
  });
});

describe("PipelineSection", () => {
  // ⚠ Rendered only when the row says one EXISTS. A clip still at `probe` has no thumbnail, and
  // firing an <img> at the hash alone would draw broken images for exactly the newest rows.
  it("renders a thumbnail only for a clip that has one", () => {
    const { container, rerender } = render(<PipelineSection rows={[row()]} ladder={LADDER} />);
    expect(container.querySelector("img")).toBeNull();

    rerender(<PipelineSection rows={[row({ thumbnail: "1985/cola.jpg" })]} ladder={LADDER} />);
    expect(container.querySelector("img")).not.toBeNull();
  });

  // ⚠ formatClipDuration, not formatDuration: the latter is minute-granular because it formats
  // programme runtimes, so a 31-second advert would render "0m". The seconds granularity IS the
  // assertion — "31s" passing where "0m" would not is the whole point of the distinction.
  it("renders the duration at clip granularity, and omits it when there is none", () => {
    const { rerender } = render(<PipelineSection rows={[row({ durationMs: 31_000 })]} ladder={LADDER} />);
    expect(screen.getByText("31s")).toBeInTheDocument();

    rerender(<PipelineSection rows={[row()]} ladder={LADDER} />);
    expect(screen.queryByText("31s")).not.toBeInTheDocument();
  });

  // The live region carries the MOST RECENT transition, chosen by updatedAt — not the first row,
  // and not one per clip.
  it("announces the most recently changed clip", () => {
    render(
      <PipelineSection
        rows={[
          row({ hash: "a", name: "Older", updatedAt: "2026-08-08T10:00:00Z" }),
          row({ hash: "b", name: "Newest", stage: "vision", updatedAt: "2026-08-08T10:05:00Z" }),
        ]}
        ladder={LADDER}
      />,
    );

    expect(screen.getByRole("status")).toHaveTextContent("Newest: Looking at the picture");
  });

  it("counts the queue in the heading", () => {
    render(<PipelineSection rows={[row(), row({ hash: "b" })]} ladder={LADDER} />);

    expect(screen.getByRole("heading", { name: /preparing 2 clips/i })).toBeInTheDocument();
  });
});
