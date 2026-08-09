import type { IncomingAskDTO, IncomingPipelineDTO, IncomingReelDTO, IncomingRejectDTO } from "@loomarr/api";

interface IncomingPanelProps {
  // The server's answer, verbatim (contract 1:1) — the two halves of one read.
  asks: IncomingAskDTO[];
  reels: IncomingReelDTO[];
  /**
   * What is still being PREPARED (§10 V51b) — the answer to "I downloaded forty clips and nothing
   * is happening". Nothing here needs the operator; it is the machine showing its work.
   */
  pipeline?: IncomingPipelineDTO[];
  /**
   * The whole stage ladder in run order — the response's `stageOrder`.
   *
   * ⚠ Required alongside `pipeline` and NOT derived from it: a row's `stages` is the VISITED
   * ladder, so a strip drawn from it would grow as a clip advances instead of filling.
   */
  stageOrder?: string[];
  /**
   * What ingest REFUSED, and why (§10 V51b) — the audit half of refusal, sibling of
   * `recentlyFiled`. Not optional in spirit: `filler.reject.unidentified` is on by default, so a
   * default that can turn down good clips has to show what it caught.
   */
  rejected?: IncomingRejectDTO[];
  /**
   * Puts a soft-rejected clip back in the catalog.
   *
   * ⚠ Rendered only when the server marked that clip `restorable`. A hard reject (no audio, no
   * video, unreadable) offers no override, because restoring it is a control that could not work.
   */
  onRestore?: (clip: IncomingRejectDTO) => void;
  /**
   * What Loomarr filed WITHOUT asking (§10 V38) — the audit half of auto-filing.
   *
   * ⚠ Not decoration. Auto-filing is on by default, so on an upgraded install clips begin
   * entering the catalog unattended; an operator who did not expect that must be able to see
   * exactly what was filed and send any of it back. Absent renders no section at all.
   */
  recentlyFiled?: IncomingAskDTO[];
  // Confirms an era the tagger guessed. ⚠ Confirming is what CLEARS the suggestion (§10 V34),
  // and it goes through the ordinary tag edit so the grounding rule has one implementation.
  onConfirmEra?: (clip: IncomingAskDTO) => void;
  // Opens the full tag editor for a clip whose guess was wrong, or which has no guess at all.
  onEditTags?: (clip: IncomingAskDTO) => void;
  // Removes a clip from the catalog. ⚠ A tombstone, never a file delete.
  onDismiss?: (clip: IncomingAskDTO) => void;
  /**
   * Files one clip as it stands — its tags are right enough, whatever the score said.
   * Distinct from `onConfirmEra`, which first CONFIRMS a guessed era.
   */
  onFile?: (clip: IncomingAskDTO) => void;
  /**
   * Files every ask, confirming each clip's OWN suggested era (the mock's "File all as
   * suggested").
   *
   * ⚠ Per clip, never one era across the selection — that is what the bulk tag bar does, and it
   * is the wrong answer for a queue of clips with different guesses.
   */
  onFileAllAsSuggested?: () => void;
  /**
   * Sends an auto-filed clip back to the queue — the undo. ⚠ It does NOT remove the clip: the
   * row and the file both stay, which is what separates it from `onDismiss`.
   */
  onSendBack?: (clip: IncomingAskDTO) => void;
  // Which clip path is currently being written, so its row can say so.
  busyPath?: string;
  className?: string;
}

export type { IncomingPanelProps };
