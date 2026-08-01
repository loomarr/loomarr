import type { SplitProposal, SplitSegment } from "@loomarr/api";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { SplitReviewEditor } from "./split-review-editor";

// Two ordinary segments plus one carrying every flag the review must surface (§10 V34):
// an ungrounded era guess, a duplicate flag, an unsplittable marker, and a transcript.
const seg = (over: Partial<SplitSegment> = {}): SplitSegment => ({
  index: 0,
  startMs: 0,
  endMs: 30000,
  name: "Segment",
  ...over,
});

const proposal: SplitProposal = {
  id: "sp-1",
  clipPath: "comp.mp4",
  createdAt: "2026-07-25T20:00:00Z",
  segments: [
    seg({
      index: 0,
      startMs: 0,
      endMs: 30000,
      name: "First ad",
      era: 1990,
      audience: "kids",
      category: "toys",
    }),
    seg({ index: 1, startMs: 30000, endMs: 61000, name: "Second ad", suggestedEra: 1985 }),
    seg({
      index: 2,
      startMs: 61000,
      endMs: 150000,
      name: "Long block",
      dupOf: "clip-gushers.mp4",
      unsplittable: true,
      transcript: "[01:01] And now a word from our sponsor.",
    }),
  ],
};

const renderEditor = (onConfirm = vi.fn(), onBack = vi.fn()) => {
  render(<SplitReviewEditor proposal={proposal} onConfirm={onConfirm} onBack={onBack} />);
  return { onConfirm, onBack };
};

// The payload the editor hands the page — this IS the body the page POSTs to /confirm,
// so these tests assert on it rather than on pixels.
const confirmed = (onConfirm: ReturnType<typeof vi.fn>): SplitSegment[] => {
  fireEvent.click(screen.getByRole("button", { name: /confirm cuts/i }));
  expect(onConfirm).toHaveBeenCalledOnce();
  return onConfirm.mock.calls[0]?.[0] as SplitSegment[];
};

describe("SplitReviewEditor", () => {
  it("renders each segment's name, mm:ss cuts, duration and tag chips", () => {
    renderEditor();
    const first = screen.getByRole("region", { name: /segment 1: first ad/i });
    expect(within(first).getByLabelText("Name")).toHaveValue("First ad");
    expect(within(first).getByLabelText("Start (mm:ss)")).toHaveValue("00:00");
    expect(within(first).getByLabelText("End (mm:ss)")).toHaveValue("00:30");
    expect(within(first).getByText("30s")).toBeInTheDocument();
    expect(within(first).getByText("1990s")).toBeInTheDocument();
    expect(within(first).getByText("Kids")).toBeInTheDocument();
    expect(within(first).getByText("toys")).toBeInTheDocument();
  });

  it("renders the duplicate flag, the unsplittable marker, and the transcript behind a toggle", () => {
    renderEditor();
    const third = screen.getByRole("region", { name: /segment 3: long block/i });
    expect(within(third).getByText(/already in the catalog: clip-gushers\.mp4/i)).toBeInTheDocument();
    expect(within(third).getByText(/couldn't see boundaries/i)).toBeInTheDocument();
    // The transcript stays collapsed until asked for.
    expect(within(third).queryByText(/word from our sponsor/)).not.toBeInTheDocument();
    fireEvent.click(within(third).getByRole("button", { name: /transcript/i }));
    expect(within(third).getByText(/word from our sponsor/)).toBeInTheDocument();
  });

  it("confirms the edited cut list — names and mm:ss times parsed back to ms", () => {
    const { onConfirm } = renderEditor();
    const first = screen.getByRole("region", { name: /segment 1: first ad/i });
    fireEvent.change(within(first).getByLabelText("Name"), { target: { value: "Sunny D" } });
    fireEvent.change(within(first).getByLabelText("Start (mm:ss)"), { target: { value: "0:02" } });
    fireEvent.change(within(first).getByLabelText("End (mm:ss)"), { target: { value: "0:32" } });

    const payload = confirmed(onConfirm);
    expect(payload).toHaveLength(3);
    expect(payload[0]).toMatchObject({ index: 0, name: "Sunny D", startMs: 2000, endMs: 32000 });
  });

  it("dropping a segment removes it from the payload and renumbers the rest", () => {
    const { onConfirm } = renderEditor();
    const first = screen.getByRole("region", { name: /segment 1: first ad/i });
    fireEvent.click(within(first).getByRole("button", { name: /^drop$/i }));

    const payload = confirmed(onConfirm);
    expect(payload).toHaveLength(2);
    expect(payload[0]).toMatchObject({ index: 0, name: "Second ad", startMs: 30000 });
    expect(payload[1]).toMatchObject({ index: 1, name: "Long block" });
  });

  it("merging with next concatenates the spans into one segment", () => {
    const { onConfirm } = renderEditor();
    const first = screen.getByRole("region", { name: /segment 1: first ad/i });
    fireEvent.click(within(first).getByRole("button", { name: /merge with next/i }));

    const payload = confirmed(onConfirm);
    expect(payload).toHaveLength(2);
    // The merged span runs from the first segment's start to the SECOND's end, keeping
    // the first's name; the second segment is gone from the list.
    expect(payload[0]).toMatchObject({ index: 0, name: "First ad", startMs: 0, endMs: 61000 });
    expect(payload[1]).toMatchObject({ index: 1, name: "Long block" });
  });

  it("accepting an era suggestion grounds it as the segment's era; rejecting clears it", () => {
    const accept = renderEditor();
    const second = screen.getByRole("region", { name: /segment 2: second ad/i });
    fireEvent.click(within(second).getByRole("button", { name: /accept 1985/i }));

    const payload = confirmed(accept.onConfirm);
    expect(payload[1]).toMatchObject({ era: 1985, suggestedEra: undefined });
  });

  it("rejecting an era suggestion drops the guess without setting an era", () => {
    const { onConfirm } = renderEditor();
    const second = screen.getByRole("region", { name: /segment 2: second ad/i });
    fireEvent.click(within(second).getByRole("button", { name: /^reject$/i }));

    const payload = confirmed(onConfirm);
    expect(payload[1]?.era).toBeUndefined();
    expect(payload[1]?.suggestedEra).toBeUndefined();
  });

  it("refuses to confirm an unparseable or inverted span", () => {
    const { onConfirm } = renderEditor();
    const first = screen.getByRole("region", { name: /segment 1: first ad/i });
    fireEvent.change(within(first).getByLabelText("End (mm:ss)"), { target: { value: "not-a-time" } });
    expect(within(first).getByText(/invalid span/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /confirm cuts/i })).toBeDisabled();
    expect(onConfirm).not.toHaveBeenCalled();
  });

  it("Back leaves without confirming", () => {
    const { onConfirm, onBack } = renderEditor();
    fireEvent.click(screen.getByRole("button", { name: /^back$/i }));
    expect(onBack).toHaveBeenCalledOnce();
    expect(onConfirm).not.toHaveBeenCalled();
  });
});
