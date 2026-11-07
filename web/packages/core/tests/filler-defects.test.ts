import { describe, expect, it } from "vitest";
import {
  defectFindingLine,
  defectFindingsToShow,
  defectJudgementLine,
  defectScoreCells,
  defectVerdictSummary,
} from "../filler-defects";

const score = {
  band: "tape" as const,
  precision: 1,
  caught: 1,
  failed: 0,
  falseFails: 0,
  judged: 0,
  missed: 0,
  borderlinePass: 0,
  judgeRight: 0,
  judgeWrong: 0,
  total: 0,
};

describe("defect report wording", () => {
  it("shows what failed first and leaves passes out", () => {
    const shown = defectFindingsToShow([
      { check: "blocks", outcome: "pass", measured: 1.1, bar: 4 },
      { check: "blur", outcome: "borderline", measured: 8.7123, bar: 8 },
      { check: "dropouts", outcome: "fail", measured: 0.00341, bar: 0.003 },
    ]);
    expect(shown.map(defectFindingLine)).toEqual([
      "Tape dropouts 0.00341, fail (bar 0.003)",
      "Softness 8.712, borderline (bar 8)",
    ]);
  });

  it("says when there was too little picture", () => {
    expect(defectFindingLine({ check: "upscale", outcome: "undecided", measured: 0, bar: 0 })).toBe(
      "Upscaled: not enough picture to judge",
    );
  });

  it("names the judge and its reason, and a missing judge plainly", () => {
    expect(defectJudgementLine({ ruling: "pass", model: "m1", reason: "soft VHS" })).toBe(
      "Judge (m1): pass, “soft VHS”",
    );
    expect(defectJudgementLine({ ruling: "abstain" })).toBe("Judge: couldn't decide");
    expect(defectJudgementLine({ ruling: "no_judge" })).toBe("No vision model was set up to rule on it");
  });

  it("summarises the verdicts, naming only the counts that are there", () => {
    expect(
      defectVerdictSummary({ verdicts: { fail: 23, pass: 180, pending: 0, undecided: 0 }, unmeasured: 0 }),
    ).toBe("Would retire 23 · pass 180");
    expect(
      defectVerdictSummary({ verdicts: { fail: 1, pass: 2, pending: 4, undecided: 1 }, unmeasured: 1 }),
    ).toBe("Would retire 1 · pass 2 · pending 4 · undecided 1 · 1 clip not measured yet");
  });

  it("scores a band, with a dash for a share over nothing", () => {
    expect(defectScoreCells(score)).toEqual({
      band: "Tape",
      labelled: "0",
      precision: "—",
      caught: "—",
      judge: "—",
    });
    expect(
      defectScoreCells({
        ...score,
        precision: 0.5,
        caught: 2 / 3,
        failed: 1,
        falseFails: 1,
        judged: 1,
        missed: 1,
        judgeRight: 3,
        judgeWrong: 1,
        total: 6,
      }),
    ).toEqual({ band: "Tape", labelled: "6", precision: "50%", caught: "67%", judge: "3 of 4" });
  });
});
