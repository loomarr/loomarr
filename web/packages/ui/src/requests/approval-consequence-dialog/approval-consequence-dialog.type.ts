import type { ApprovalChannelEffect, SizedItem } from "@loomarr/core/requests";

/**
 * One selected channel request, reduced to what approving it will DO (#1817). Every figure is the
 * server's: counts from the proposal, sizes from its items (#1885), the channel from the job
 * binding the server itself uses.
 */
interface ChannelRequestConsequence {
  id: string;
  title: string;
  titleCount: number;
  toDownload: number;
  /** The proposal's audience ceiling; absent when it has none. */
  ceiling?: string | undefined;
  /** Every lineup + acquisition item. Owned movies carry a size; the rest are absent, never 0. */
  sizes: readonly SizedItem[];
  channel: ApprovalChannelEffect;
}

/**
 * One selected filler download. ⚠ No size and no channel: nothing writes either since #1846
 * removed the pull producer, so both render as unavailable until it returns (beta.11).
 */
interface FillerDownloadConsequence {
  id: string;
  title: string;
  /** An ESTIMATE; 0 means nothing was measured, not "no clips". */
  estimateClips: number;
  sources: number;
}

/** The two groups never mix (R1): one dialog approves exactly one group's selection. */
type BulkApproveConsequences =
  | { group: "channels"; items: readonly ChannelRequestConsequence[] }
  | { group: "filler"; items: readonly FillerDownloadConsequence[] };

/**
 * One row of either bulk endpoint's result, which share this shape. `enqueued` is the exact count
 * the approval committed (titles, or the pull's clips); absent on a failure.
 */
interface BulkApproveOutcome {
  id: string;
  ok: boolean;
  error?: string | undefined;
  enqueued?: number | undefined;
}

interface ApprovalConsequenceDialogProps {
  visible: boolean;
  consequences: BulkApproveConsequences;
  /** Present once the bulk call has answered: the dialog shows the result step instead. */
  results?: readonly BulkApproveOutcome[] | undefined;
  /** An approve or retry call is in flight. */
  pending?: boolean | undefined;
  /** Why the whole call failed (no per-item results to show), already worded for a person. */
  error?: string | undefined;
  onCancel: () => void;
  onConfirm: () => void;
  /** Re-submit only these ids through the same bulk endpoint. */
  onRetry: (ids: string[]) => void;
  onDone: () => void;
}

export type {
  ApprovalConsequenceDialogProps,
  BulkApproveConsequences,
  BulkApproveOutcome,
  ChannelRequestConsequence,
  FillerDownloadConsequence,
};
