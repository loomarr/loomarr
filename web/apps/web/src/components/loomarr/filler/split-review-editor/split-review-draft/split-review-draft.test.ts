import type { SplitReviewSegmentDTO } from "@loomarr/api/models/splitReviewSegmentDTO";
import { describe, expect, it } from "vitest";
import {
  isValid,
  languageNeedsRecheck,
  nameDetail,
  resolveMs,
  spanMs,
  toDraft,
  toWire,
  unresolvedLanguageMessage,
} from "./split-review-draft";

const seg = (over: Partial<SplitReviewSegmentDTO> = {}): SplitReviewSegmentDTO => ({
  index: 0,
  startMs: 0,
  endMs: 30_000,
  name: "Segment",
  ...over,
});

describe("split review draft", () => {
  it("renders each boundary as mm:ss text with a stable key", () => {
    const draft = toDraft(seg({ index: 3, startMs: 61_000, endMs: 90_500 }));
    expect(draft).toMatchObject({ key: "seg-3-61000", startText: "01:01", endText: "01:30" });
  });

  it("keeps an untouched sub-second boundary and takes a typed one at mm:ss precision", () => {
    expect(resolveMs("00:01", 1_500)).toBe(1_500);
    expect(resolveMs("00:25", 30_499)).toBe(25_000);
    // A half-typed field falls back to the committed value, never to zero.
    expect(resolveMs("1:", 30_499)).toBe(30_499);
  });

  it("renumbers the wire body by draft order and strips the editing fields", () => {
    const first = toDraft(seg({ index: 4, startMs: 1_500, endMs: 30_499, name: "B" }));
    const second = {
      ...toDraft(seg({ index: 9, startMs: 30_499, endMs: 61_750, name: "A" })),
      endText: "01:00",
    };
    const wire = toWire([first, second]);
    expect(wire).toEqual([
      expect.objectContaining({ index: 0, name: "B", startMs: 1_500, endMs: 30_499 }),
      expect.objectContaining({ index: 1, name: "A", startMs: 30_499, endMs: 60_000 }),
    ]);
    for (const segment of wire) {
      expect(segment).not.toHaveProperty("key");
      expect(segment).not.toHaveProperty("startText");
      expect(segment).not.toHaveProperty("endText");
      expect(segment).not.toHaveProperty("artwork");
    }
  });

  it("accepts only a named span of at least three seconds that ends after it starts", () => {
    const ok = toDraft(seg({ endMs: 3_000 }));
    expect(spanMs(ok)).toBe(3_000);
    expect(isValid(ok)).toBe(true);
    expect(isValid(toDraft(seg({ endMs: 2_999 })))).toBe(false);
    expect(isValid({ ...ok, name: "  " })).toBe(false);
    expect(isValid({ ...ok, startText: "00:10" })).toBe(false);
    const unparseable = { ...ok, endText: "not-a-time" };
    expect(spanMs(unparseable)).toBeUndefined();
    expect(isValid(unparseable)).toBe(false);
  });

  it("explains where a name came from", () => {
    expect(nameDetail(seg({ nameOrigin: "model-proposed", nameEvidence: "FIZZBOLT" }))).toBe(
      "Name suggested from this clip: “FIZZBOLT”",
    );
    expect(nameDetail(seg({ nameOrigin: "model-proposed" }))).toBe("Name suggested from this clip.");
    expect(nameDetail(seg({ nameOrigin: "source-authored" }))).toBe("Name provided by the source.");
    expect(nameDetail(seg({ nameOrigin: "fallback" }))).toMatch(/temporary name/i);
    expect(nameDetail(seg({ nameOrigin: "operator-edited" }))).toMatch(/edited during review/i);
    expect(nameDetail(seg())).toBeUndefined();
  });

  it("says why a language is unresolved, and nothing when there is no reason", () => {
    expect(unresolvedLanguageMessage(seg({ languageReason: "unavailable" }))).toMatch(/isn’t set up/);
    expect(unresolvedLanguageMessage(seg({ languageReason: "failed" }))).toMatch(/had a problem/);
    expect(unresolvedLanguageMessage(seg({ languageReason: "inconclusive" }))).toMatch(/confidently/);
    expect(unresolvedLanguageMessage(seg({ languageReason: "paused" }))).toMatch(/paused/);
    expect(unresolvedLanguageMessage(seg({ languageNote: languageNeedsRecheck.languageNote }))).toBe(
      languageNeedsRecheck.languageNote,
    );
    expect(unresolvedLanguageMessage(seg({ languageNote: "anything else" }))).toMatch(
      /couldn’t be confirmed/,
    );
    expect(unresolvedLanguageMessage(seg())).toBeUndefined();
  });
});
