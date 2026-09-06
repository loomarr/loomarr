import type { IncomingClipDTO } from "@loomarr/api/models/incomingClipDTO";
import type { IncomingReelDTO } from "@loomarr/api/models/incomingReelDTO";
import type { IncomingRejectDTO } from "@loomarr/api/models/incomingRejectDTO";

interface IncomingPanelProps {
  /**
   * The conveyor, verbatim from the server (contract 1:1): every clip downloaded and not yet
   * filed, whether the machine is still preparing it or has handed it over. Ordered
   * decisions-first.
   *
   * ⚠ ONE list. It was `asks` + `pipeline`, two arrays over overlapping populations, and the same
   * clip rendered twice — once demanding a decision, once captioned "nothing here needs you".
   * `needsDecision` on the row is what this panel branches on.
   */
  clips: IncomingClipDTO[];
  clipsTotal?: number;
  decisionsTotal?: number;
  reels: IncomingReelDTO[];
  reelsTotal?: number;
  /** Another part of the same workbench already renders current work, so this list is not globally empty. */
  suppressEmptyState?: boolean;
  /**
   * The whole stage ladder in run order — the response's `stageOrder`.
   *
   * ⚠ NOT derived from a row's `stages`, which is the VISITED ladder: a strip drawn from that
   * would grow as a clip advances instead of filling.
   */
  stageOrder?: string[];
  /**
   * What ingest REFUSED, and why (§10 V51b). Not optional in spirit:
   * `filler.reject.unidentified` is on by default, so a
   * default that can turn down good clips has to show what it caught.
   */
  rejected?: IncomingRejectDTO[];
  rejectedTotal?: number;
  /**
   * Puts a soft-rejected clip back in the catalog.
   *
   * ⚠ Rendered only when the server marked that clip `restorable`. A hard reject (no audio, no
   * video, unreadable) offers no override, because restoring it is a control that could not work.
   */
  onRestore?: (clip: IncomingRejectDTO) => void;
  /** Retries a server-classified execution failure at its exact failed stage. */
  onRetryFailure?: (clip: IncomingRejectDTO) => void;
  // Opens the full tag editor for a clip whose guess was wrong, or which has no guess at all.
  onEditTags?: (clip: IncomingClipDTO) => void;
  /** Re-run the classifier after its provider/configuration was fixed. This preserves the clip
   * and upstream pipeline work; the server resets tag and dependent stages only. */
  onReclassify?: (clip: IncomingClipDTO) => void;
  // Removes a clip from the catalog. ⚠ A tombstone, never a file delete.
  onDismiss?: (clip: IncomingClipDTO) => void;
  /** Retry a server-approved failed stage immediately, preserving completed upstream work. */
  onRetryStage?: (clip: IncomingClipDTO, stage: string) => void;
  // Which clip path is currently being written, so its row can say so.
  busyPath?: string;
  className?: string;
}

export type { IncomingPanelProps };
