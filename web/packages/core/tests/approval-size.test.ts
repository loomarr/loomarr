import { SizeConfidence } from "@loomarr/api/models/sizeConfidence";
import { describe, expect, it } from "vitest";
import { formatBytes } from "../format";
import {
  aggregateSize,
  aggregateSizeLabel,
  approvalSizeLine,
  itemSizeLabel,
  SIZE_UNAVAILABLE,
} from "../requests";

const GiB = 1024 ** 3;
const exact = (bytes: number) => ({ sizeBytes: bytes, sizeConfidence: SizeConfidence.exact });
const estimated = (bytes: number) => ({ sizeBytes: bytes, sizeConfidence: SizeConfidence.estimated });
const unknown = {};

describe("formatBytes — one shared rounding for every size on screen", () => {
  it.each([
    [0, "0 B"],
    [1023, "1023 B"],
    [1024, "1.0 KB"],
    [1024 ** 2 - 1, "1.0 MB"], // rounds up into the next unit, never "1024.0 KB"
    [1024 ** 2, "1.0 MB"],
    [1024 ** 3 - 1, "1.0 GB"],
    [2.1 * GiB, "2.1 GB"],
    [1024 ** 4 - 1, "1.0 TB"],
    [1024 ** 4, "1.0 TB"],
    [3.5 * 1024 ** 4, "3.5 TB"],
    [2048 * 1024 ** 4, "2048.0 TB"], // TB is the ceiling; there is no larger unit to roll into
  ])("%d bytes → %s", (bytes, want) => {
    expect(formatBytes(bytes)).toBe(want);
  });
});

describe("itemSizeLabel — the wording the design pass fixed (#1817)", () => {
  it("states an exact size plainly", () => {
    expect(itemSizeLabel(exact(2.1 * GiB))).toBe("2.1 GB");
  });

  it("marks an estimate with both ~ and (estimated)", () => {
    expect(itemSizeLabel(estimated(2.1 * GiB))).toBe("~2.1 GB (estimated)");
  });

  it("says unavailable for an absent size, never 0 B", () => {
    expect(itemSizeLabel(unknown)).toBe("estimate unavailable");
    expect(itemSizeLabel(unknown)).toBe(SIZE_UNAVAILABLE);
  });

  // The contract says absent, never 0 — a stray 0 must not become the "0 B" the rule forbids.
  it("treats a zero or negative byte count as unknown", () => {
    expect(itemSizeLabel(exact(0))).toBe(SIZE_UNAVAILABLE);
    expect(itemSizeLabel(exact(-5))).toBe(SIZE_UNAVAILABLE);
  });

  it("does not let bytes without a confidence claim to be exact", () => {
    expect(itemSizeLabel({ sizeBytes: 2.1 * GiB })).toBe("~2.1 GB (estimated)");
  });
});

describe("aggregateSize — sums only what is known and counts the rest", () => {
  it("sums every known item when all are known", () => {
    expect(aggregateSize([exact(GiB), exact(2 * GiB)])).toEqual({
      bytes: 3 * GiB,
      known: 2,
      total: 2,
      estimated: false,
    });
  });

  it("leaves unknown items out of the sum but in the total", () => {
    expect(aggregateSize([exact(GiB), unknown, unknown])).toEqual({
      bytes: GiB,
      known: 1,
      total: 3,
      estimated: false,
    });
  });

  it("is an estimate as soon as one contributing item is", () => {
    expect(aggregateSize([exact(GiB), estimated(GiB)]).estimated).toBe(true);
  });

  // An unknown item contributes nothing to the sum, so it cannot weaken its confidence either.
  it("an unknown item does not turn an exact sum into an estimate", () => {
    expect(aggregateSize([exact(GiB), unknown]).estimated).toBe(false);
  });
});

describe("aggregateSizeLabel — never a partial sum dressed as a total", () => {
  it("states the sum when every size is known", () => {
    expect(aggregateSizeLabel(aggregateSize([exact(4 * GiB), exact(4.4 * GiB)]), "titles")).toBe("8.4 GB");
  });

  it("marks an all-known sum with an estimate in it as estimated", () => {
    expect(aggregateSizeLabel(aggregateSize([exact(4 * GiB), estimated(4.4 * GiB)]), "titles")).toBe(
      "~8.4 GB (estimated)",
    );
  });

  it("names the coverage and the unknown count when only some are known", () => {
    const items = [
      ...Array.from({ length: 4 }, () => exact(2.1 * GiB)),
      ...Array.from({ length: 10 }, () => unknown),
    ];
    expect(aggregateSizeLabel(aggregateSize(items), "titles")).toBe("8.4 GB for 4 of 14 titles · 10 unknown");
  });

  it("keeps the estimate marks on a partial sum", () => {
    expect(aggregateSizeLabel(aggregateSize([estimated(GiB), unknown]), "titles")).toBe(
      "~1.0 GB (estimated) for 1 of 2 titles · 1 unknown",
    );
  });

  it("says unavailable when nothing is known, rather than 0 B", () => {
    expect(aggregateSizeLabel(aggregateSize([unknown, unknown]), "titles")).toBe(SIZE_UNAVAILABLE);
  });

  it("says unavailable for an empty selection", () => {
    expect(aggregateSizeLabel(aggregateSize([]), "titles")).toBe(SIZE_UNAVAILABLE);
  });
});

describe("approvalSizeLine — the line every surface prints", () => {
  it.each([
    [[exact(2.1 * GiB)], "Size: 2.1 GB"],
    [[estimated(2.1 * GiB)], "Size: ~2.1 GB (estimated)"],
    [[unknown], "Size: estimate unavailable"],
    [[exact(GiB), unknown], "Size: 1.0 GB for 1 of 2 titles · 1 unknown"],
  ])("%j → %s", (items, want) => {
    expect(approvalSizeLine(items, "titles")).toBe(want);
  });
});
