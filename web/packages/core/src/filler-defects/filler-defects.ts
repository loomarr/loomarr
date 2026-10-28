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

// Readings span shares of 0.0005 and edge widths of 9, so four significant figures keep both legible.
const reading = (n: number): string => String(Number(n.toPrecision(4)));

const defectFindingLine = (f: FillerDefectFindingDTO): string =>
  f.outcome === "undecided"
    ? `${defectCheckLabel(f.check)}: not enough picture to judge`
    : `${defectCheckLabel(f.check)} ${reading(f.measured)}, ${f.outcome} (bar ${reading(f.bar)})`;

const defectJudgementLine = (j: FillerDefectJudgementDTO): string => {
  if (j.ruling === "no_judge") return "No vision model was set up to rule on it";
  const who = j.model ? `Judge (${j.model})` : "Judge";
  const ruling = j.ruling === "abstain" ? "couldn't decide" : j.ruling;
  return j.reason ? `${who}: ${ruling}, “${j.reason}”` : `${who}: ${ruling}`;
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
