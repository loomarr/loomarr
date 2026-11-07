import type { FillerDefectReportDTO } from "@loomarr/core/filler-defects";
import type { ReactNode } from "react";

type DefectReviewProps = {
  /** The report's first unlabelled page: its summary and scores cover every record, its first row is the clip to label. */
  report: FillerDefectReportDTO;
  /** The host's player for the first row's clip. */
  player: ReactNode;
  /** A label is being saved: the buttons hold still rather than label the next clip twice. */
  busy?: boolean;
  /** Why the last label wasn't saved. */
  error?: string;
  onLabel: (clipId: string, label: "pass" | "fail") => void;
  onSkip: () => void;
};

export type { DefectReviewProps };
