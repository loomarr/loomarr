import type { FillerDefectBandScoreDTO } from "@loomarr/api/models/fillerDefectBandScoreDTO";
import type { FillerDefectFindingDTO } from "@loomarr/api/models/fillerDefectFindingDTO";
import type { FillerDefectFindingDTOCheck } from "@loomarr/api/models/fillerDefectFindingDTOCheck";
import type { FillerDefectJudgementDTO } from "@loomarr/api/models/fillerDefectJudgementDTO";
import type { FillerDefectReportDTO } from "@loomarr/api/models/fillerDefectReportDTO";
import type { FillerDefectRowDTO } from "@loomarr/api/models/fillerDefectRowDTO";
import type { FillerDefectRowDTOBand } from "@loomarr/api/models/fillerDefectRowDTOBand";
import { pluralize } from "../format";

// Wording for the defect floor's household report (#2034): what each check means to someone
// watching, and the floor's scores against the maintainer's labels. Web and native share it.

const CHECK_LABELS: Record<FillerDefectFindingDTOCheck, string> = {
  blocks: "Compression blocks",
  blur: "Softness",
  dropouts: "Tape dropouts",
  interlace: "Combing",
  windowbox: "Windowboxed",
  upscale: "Upscaled",
  clipping: "Clipped sound",
};

const BAND_LABELS: Record<FillerDefectRowDTOBand, string> = {
  tape: "Tape",
  sd: "SD",
  hd: "HD",
};

const defectCheckLabel = (check: FillerDefectFindingDTOCheck): string => CHECK_LABELS[check];

const defectBandLabel = (band: FillerDefectRowDTOBand): string => BAND_LABELS[band];

const findingRank: Record<FillerDefectFindingDTO["outcome"], number> = {
  fail: 0,
  borderline: 1,
  undecided: 2,
  pass: 3,
};

// The findings worth a look, worst first. A pass is left out: it is what most checks read.
const defectFindingsToShow = (findings: readonly FillerDefectFindingDTO[]): FillerDefectFindingDTO[] =>
  findings
    .filter((f) => f.outcome !== "pass")
    .sort((a, b) => findingRank[a.outcome] - findingRank[b.outcome]);

// What a viewer would see, in words: a plain sentence per check for a clear fault and for one that is
// only close to the line (#2741). The measurements and bars behind it are not for the household.
const FINDING_WORDS: Record<FillerDefectFindingDTOCheck, { fail: string; borderline: string }> = {
  blocks: { fail: "Blocky picture", borderline: "Slightly blocky picture" },
  blur: { fail: "Soft picture", borderline: "Slightly soft picture" },
  dropouts: { fail: "Tape dropouts", borderline: "A few tape dropouts" },
  interlace: { fail: "Combing lines", borderline: "Faint combing lines" },
  windowbox: { fail: "Windowboxed", borderline: "Possibly windowboxed" },
  upscale: {
    fail: "Upscaled from a smaller picture",
    borderline: "Possibly upscaled from a smaller picture",
  },
  clipping: { fail: "Clipped sound", borderline: "Slightly clipped sound" },
};

const defectFindingLine = (f: FillerDefectFindingDTO): string => {
  if (f.outcome === "undecided") return `${defectCheckLabel(f.check)}: not enough picture to judge`;
  if (f.outcome === "pass") return `${defectCheckLabel(f.check)} looks fine`;
  return FINDING_WORDS[f.check][f.outcome];
};

const defectJudgementLine = (j: FillerDefectJudgementDTO): string => {
  if (j.ruling === "no_judge") return "No vision model was set up to rule on it";
  const ruling = j.ruling === "abstain" ? "couldn't decide" : j.ruling === "pass" ? "it passes" : "it fails";
  const verdict = `A second look by a vision model: ${ruling}`;
  return j.reason ? `${verdict}, “${j.reason}”` : verdict;
};

const defectVerdictSummary = (report: Pick<FillerDefectReportDTO, "verdicts" | "unmeasured">): string =>
  [
    `Would retire ${report.verdicts.fail}`,
    `pass ${report.verdicts.pass}`,
    ...(report.verdicts.pending ? [`pending ${report.verdicts.pending}`] : []),
    ...(report.verdicts.undecided ? [`undecided ${report.verdicts.undecided}`] : []),
    ...(report.unmeasured ? [`${pluralize(report.unmeasured, "clip")} not measured yet`] : []),
  ].join(" · ");

type DefectScoreCells = { band: string; labelled: string; precision: string; caught: string; judge: string };

const percent = (share: number): string => `${Math.round(share * 100)}%`;

// One row of the score table. A share over nothing reads as a dash, not the 100% the server
// reports for an empty band, so a fresh install does not look perfectly calibrated.
const defectScoreCells = (s: FillerDefectBandScoreDTO): DefectScoreCells => {
  const outrightFails = s.failed + s.falseFails;
  const labelledFail = s.failed + s.judged + s.missed;
  const rulings = s.judgeRight + s.judgeWrong;
  return {
    band: defectBandLabel(s.band),
    labelled: String(s.total),
    precision: outrightFails ? percent(s.precision) : "—",
    caught: labelledFail ? percent(s.caught) : "—",
    judge: rulings ? `${s.judgeRight} of ${rulings}` : "—",
  };
};

export type { DefectScoreCells, FillerDefectReportDTO, FillerDefectRowDTO };
export {
  defectBandLabel,
  defectCheckLabel,
  defectFindingLine,
  defectFindingsToShow,
  defectJudgementLine,
  defectScoreCells,
  defectVerdictSummary,
};
