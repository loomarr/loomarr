import type { IncomingPipelineDTO } from "@loomarr/api";

/** `strip` is the 8-pip summary for a collapsed row; `list` is the named, explained ladder. */
type ClipPipelineVariant = "strip" | "list";

interface ClipPipelineProps {
  /** The clip's pipeline block, verbatim from `GET /v1/filler/incoming`. */
  row: IncomingPipelineDTO;
  /**
   * The clip's display name, for the list's accessible name.
   *
   * ⚠ Passed in rather than read off `row`: the pipeline block carries no identity or display
   * fields, because it is nested inside the clip that already has them. The version that repeated
   * them also did a `GetClip` per row on the server to fill them in.
   */
  name: string;
  /**
   * The whole ladder in run order — the response's `stageOrder`.
   *
   * ⚠ Required, and NOT derived from `row.stages`. That field is the VISITED ladder, so a clip at
   * `split` carries three records; drawing one pip per record would make the strip grow as the
   * clip advances, and the operator could never see how far there is left to go.
   */
  ladder: string[];
  variant?: ClipPipelineVariant;
  className?: string;
}

export type { ClipPipelineProps, ClipPipelineVariant };
