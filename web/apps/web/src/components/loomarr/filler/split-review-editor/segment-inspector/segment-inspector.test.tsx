import type { SplitReviewSegmentDTO } from "@loomarr/api/models/splitReviewSegmentDTO";
import { fireEvent, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { render } from "@/test/render";
import { toDraft } from "../split-review-draft";
import { SegmentInspector } from "./segment-inspector";

const seg = (over: Partial<SplitReviewSegmentDTO> = {}): SplitReviewSegmentDTO => ({
  index: 0,
  startMs: 0,
  endMs: 30_000,
  name: "First ad",
  ...over,
});

const renderInspector = (
  over: Partial<SplitReviewSegmentDTO> = {},
  props: { last?: boolean; minClipDurationMs?: number } = {},
) => {
  const onChange = vi.fn();
  const onDrop = vi.fn();
  const onMergeWithNext = vi.fn();
  render(
    <SegmentInspector
      segment={toDraft(seg(over))}
      position={0}
      last={props.last ?? false}
      clipHash="comp-hash"
      minClipDurationMs={props.minClipDurationMs}
      onChange={onChange}
      onDrop={onDrop}
      onMergeWithNext={onMergeWithNext}
    />,
  );
  return { onChange, onDrop, onMergeWithNext };
};

describe("SegmentInspector", () => {
  it("shows the span, the tag chips and one preview for the clip", () => {
    renderInspector({ era: 1990, audience: "kids", category: "toys", tags: ["toys", "cereal"] });
    expect(screen.getByRole("region", { name: "Clip 1: First ad" })).toBeInTheDocument();
    expect(screen.getByText(/00:00–00:30 · 30s/)).toBeInTheDocument();
    expect(screen.getByText("1990s")).toBeInTheDocument();
    expect(screen.getByText("Kids")).toBeInTheDocument();
    expect(screen.getByText("toys")).toBeInTheDocument();
    expect(screen.getByText("+1")).toBeInTheDocument();
    expect(document.body.querySelectorAll("video")).toHaveLength(1);
  });

  it("edits the name and the cut points as operator-owned changes", () => {
    const { onChange } = renderInspector({ nameOrigin: "model-proposed", nameEvidence: "FIZZBOLT" });
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Fizzbolt" } });
    expect(onChange).toHaveBeenLastCalledWith({
      name: "Fizzbolt",
      nameOrigin: "operator-edited",
      nameEvidence: undefined,
    });
    fireEvent.change(screen.getByLabelText("Start (mm:ss)"), { target: { value: "0:02" } });
    expect(onChange).toHaveBeenLastCalledWith(
      expect.objectContaining({ startText: "0:02", artwork: undefined, languageChecked: false }),
    );
    fireEvent.change(screen.getByLabelText("End (mm:ss)"), { target: { value: "0:40" } });
    expect(onChange).toHaveBeenLastCalledWith(expect.objectContaining({ endText: "0:40", language: "" }));
  });

  it("offers Join next except on the last clip, and Remove always", () => {
    const { onDrop, onMergeWithNext } = renderInspector();
    fireEvent.click(screen.getByRole("button", { name: /join next/i }));
    fireEvent.click(screen.getByRole("button", { name: /^remove$/i }));
    expect(onMergeWithNext).toHaveBeenCalledOnce();
    expect(onDrop).toHaveBeenCalledOnce();
  });

  it("hides Join next on the last clip", () => {
    renderInspector({}, { last: true });
    expect(screen.queryByRole("button", { name: /join next/i })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^remove$/i })).toBeInTheDocument();
  });

  it("grounds or rejects an era suggestion", () => {
    const { onChange } = renderInspector({ suggestedEra: 1985 });
    fireEvent.click(screen.getByRole("button", { name: /use 1985/i }));
    expect(onChange).toHaveBeenLastCalledWith({ era: 1985, suggestedEra: undefined });
    fireEvent.click(screen.getByRole("button", { name: /not right/i }));
    expect(onChange).toHaveBeenLastCalledWith({ suggestedEra: undefined });
  });

  it("opens timing for an invalid span and warns below the catalog floor", () => {
    renderInspector({ endMs: 8_000 }, { minClipDurationMs: 10_000 });
    expect(screen.getByRole("status")).toHaveTextContent(/shorter than your 10s minimum/i);
    expect(screen.getByLabelText("Name").closest("details")).toHaveAttribute("open");
  });

  it("keeps timing closed for a valid clip", () => {
    renderInspector();
    expect(screen.getByLabelText("Name").closest("details")).not.toHaveAttribute("open");
  });

  it("explains an automatic hold, a missed cut, a duplicate and the cut evidence", () => {
    renderInspector({
      holdReason: "a segment could not be classified",
      unsplittable: true,
      dupOf: "clip-gloopers.mp4",
      boundaryConfidence: 90,
      startEvidence: "reel edge",
      endEvidence: "black + silence",
      transcript: "And now a word from our sponsor.",
    });
    expect(screen.getByText(/take a closer look: a segment could not be classified/i)).toBeInTheDocument();
    expect(screen.getByText(/may have missed a cut/i)).toBeInTheDocument();
    fireEvent.click(screen.getByText(/^details$/i));
    expect(screen.getByText(/already in your library: clip-gloopers\.mp4/i)).toBeInTheDocument();
    expect(screen.getByText(/cut confidence 90%/i)).toHaveTextContent(
      /start: reel edge · end: black \+ silence/,
    );
    expect(screen.getByText(/word from our sponsor/)).toBeInTheDocument();
  });

  it("shows no Details section when there is nothing to explain", () => {
    renderInspector();
    expect(screen.queryByText(/^details$/i)).not.toBeInTheDocument();
  });
});
