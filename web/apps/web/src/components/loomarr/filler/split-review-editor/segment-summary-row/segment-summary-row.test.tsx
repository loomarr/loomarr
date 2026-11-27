import type { SplitReviewSegmentDTO } from "@loomarr/api/models/splitReviewSegmentDTO";
import { fireEvent, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { render } from "@/test/render";
import { type DraftSegment, toDraft } from "../split-review-draft";
import { SegmentSummaryRow } from "./segment-summary-row";

const draft = (over: Partial<SplitReviewSegmentDTO> = {}): DraftSegment =>
  toDraft({ index: 0, startMs: 0, endMs: 30_000, name: "First ad", ...over });

const renderRow = (segment: DraftSegment, props: { selected?: boolean; minClipDurationMs?: number } = {}) => {
  const onOpen = vi.fn();
  render(
    <ol>
      <SegmentSummaryRow
        segment={segment}
        position={1}
        minClipDurationMs={props.minClipDurationMs}
        selected={props.selected ?? false}
        onOpen={onOpen}
      />
    </ol>,
  );
  return { onOpen };
};

describe("SegmentSummaryRow", () => {
  it("names the clip by its position, name, span and tags, and opens it", () => {
    const { onOpen } = renderRow(
      draft({ era: 1990, audience: "late_night", category: "toys", language: "none", languageChecked: true }),
    );
    const row = screen.getByRole("button", { name: /open clip .*#2.*first ad/i });
    expect(row).toHaveTextContent("00:00–00:30 · 30s");
    expect(row).toHaveTextContent("1990s");
    expect(row).toHaveTextContent("Late night");
    expect(row).toHaveTextContent("toys");
    expect(row).toHaveTextContent("No speech");
    expect(row).toHaveTextContent("Preview unavailable");
    expect(row).not.toHaveAttribute("aria-current");
    expect(screen.queryByText("Needs a closer look")).not.toBeInTheDocument();
    fireEvent.click(row);
    expect(onOpen).toHaveBeenCalledWith(row);
  });

  it("marks the selected clip as current", () => {
    renderRow(draft(), { selected: true });
    expect(screen.getByRole("button", { name: /open clip/i })).toHaveAttribute("aria-current", "true");
  });

  it("flags an unnamed or invalid span, a missed cut, a hold and a short clip", () => {
    renderRow({ ...draft({ name: "" }), endText: "not-a-time" });
    expect(screen.getByRole("button", { name: /unnamed clip/i })).toHaveTextContent("invalid span");
    expect(screen.getByText("Needs a closer look")).toBeInTheDocument();
  });

  it.each([
    ["a missed cut", draft({ unsplittable: true }), undefined],
    ["an automatic hold", draft({ holdReason: "could not be classified" }), undefined],
    ["a clip below the floor", draft({ endMs: 8_000 }), 10_000],
  ])("asks for a closer look at %s", (_, segment, minClipDurationMs) => {
    renderRow(segment, { minClipDurationMs });
    expect(screen.getByText("Needs a closer look")).toBeInTheDocument();
  });
});
