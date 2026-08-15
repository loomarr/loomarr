import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { beginTune, markTunePhase } from "./tuner-timing";

describe("tuner timing", () => {
  const mark = vi.fn();
  const measure = vi.fn();

  beforeEach(() => {
    mark.mockReset();
    measure.mockReset();
    vi.stubGlobal("performance", { mark, measure });
  });
  afterEach(() => vi.unstubAllGlobals());

  it("uses stable measure names without channel identity and carries attempt metadata", () => {
    const attempt = beginTune(true);
    markTunePhase(attempt, "osd");

    expect(mark).toHaveBeenCalledWith(`loomarr:tune:${attempt.id}:request`);
    expect(measure).toHaveBeenCalledWith(
      "loomarr:tune:request-to-osd",
      expect.objectContaining({ detail: { attemptId: attempt.id, adjacent: true } }),
    );
  });
});
