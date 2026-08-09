import type { IncomingClipDTO } from "@loomarr/api";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { PreparingRow, sentenceFor } from "./preparing-row";

const LADDER = ["probe", "transcode", "split", "language", "transcribe", "tag", "vision", "score"];

// ⚠ No `needsDecision`: this row only ever renders for a clip the MACHINE still owns. Its sibling
// in `incoming-panel.tsx` renders the other end of the belt, and the panel picks between them —
// which is the arrangement that stopped one clip appearing in two lists at once.
const clipAt = (over: Partial<IncomingClipDTO> = {}): IncomingClipDTO => ({
  hash: "hash-cola",
  path: "cola.mp4",
  name: "Coca-Cola 1985",
  kind: "commercial",
  durationMs: 31_000,
  reason: "Loomarr is still working on this one.",
  pipeline: {
    stage: "tag",
    status: "running",
    progress: -1,
    stages: [{ stage: "probe", status: "done", at: "2026-08-08T10:00:00Z" }],
    updatedAt: "2026-08-08T10:01:00Z",
  },
  ...over,
});

// `sentenceFor` is what the operator actually reads on a collapsed row, so its wording is tested
// directly — each branch is a different claim about what the machine is doing.
describe("sentenceFor", () => {
  it("uses the active voice for the rung being worked on", () => {
    expect(sentenceFor({ stage: "tag", status: "running" })).toBe("Working out what it is");
  });

  it("says a queued clip is waiting rather than working", () => {
    expect(sentenceFor({ stage: "vision", status: "queued" })).toBe("Waiting to look at the picture");
  });

  // ⚠ "retrying" is the honest word: a failed rung is not terminal for the clip — the runner
  // retries it, and the pipeline row only rejects once the attempts are spent.
  it("says a failed rung is being retried", () => {
    expect(sentenceFor({ stage: "transcode", status: "failed" })).toBe("Level the sound — failed, retrying");
  });

  // A rung this build has no copy for comes from a newer backend; the raw id beats a placeholder.
  it("falls back to the server's id for an unknown stage", () => {
    expect(sentenceFor({ stage: "fingerprint", status: "running" })).toBe("fingerprint");
  });
});

describe("PreparingRow", () => {
  // ⚠ Rendered only when the clip says one EXISTS. A clip still at `probe` has no thumbnail, and
  // firing an <img> at the hash alone would draw broken images for exactly the newest rows.
  it("renders a thumbnail only for a clip that has one", () => {
    const { container, rerender } = render(<PreparingRow clip={clipAt()} ladder={LADDER} />);
    expect(container.querySelector("img")).toBeNull();

    rerender(<PreparingRow clip={clipAt({ thumbnail: "1985/cola.jpg" })} ladder={LADDER} />);
    expect(container.querySelector("img")).not.toBeNull();
  });

  // ⚠ formatClipDuration, not formatDuration: the latter is minute-granular because it formats
  // programme runtimes, so a 31-second advert would render "0m". The seconds granularity IS the
  // assertion — "31s" passing where "0m" would not is the whole point of the distinction.
  it("renders the duration at clip granularity, and omits it when there is none", () => {
    const { rerender } = render(<PreparingRow clip={clipAt()} ladder={LADDER} />);
    expect(screen.getByText("31s")).toBeInTheDocument();

    rerender(<PreparingRow clip={clipAt({ durationMs: 0 })} ladder={LADDER} />);
    expect(screen.queryByText("31s")).not.toBeInTheDocument();
  });

  // ⚠ `getAllByText`, because the string legitimately appears twice: once as the collapsed row's
  // caption and once inside the expanded ladder, which `hiddenUntilFound` keeps in the DOM. The
  // assertion is that the FIRST one — the collapsed summary — is the visible one.
  it("says what is happening in the operator's words", () => {
    render(<PreparingRow clip={clipAt()} ladder={LADDER} />);

    const [caption] = screen.getAllByText("Working out what it is");
    expect(caption).toBeVisible();
  });

  // ⚠ A clip with no pipeline block must render NOTHING rather than an empty shell. It reaches
  // this component only by mistake — the panel routes on `needsDecision` — and a row with a
  // chevron, a name and no strip would look like a clip whose progress had been lost.
  it("renders nothing for a clip with no pipeline block", () => {
    const { container } = render(<PreparingRow clip={clipAt({ pipeline: undefined })} ladder={LADDER} />);

    expect(container).toBeEmptyDOMElement();
  });
});
