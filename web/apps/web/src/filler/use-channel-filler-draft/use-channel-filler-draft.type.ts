import type { FillerSelection, PodPoolDTO } from "@loomarr/api";

// The draft the sandbox edits. It is exactly a FillerSelection — the same shape the
// backend persists on `policy.filler` and previews at POST …/pods/preview — so "apply"
// is a straight handoff with no translation. Every field optional; an empty draft means
// "any" (today's behaviour).
type FillerDraft = FillerSelection;

type ChannelFillerDraft = {
  // The live draft the criteria/pin/exclude controls mutate.
  draft: FillerDraft;
  // Replace the whole draft (the controls compute the next draft and hand it back, the
  // same controlled-parent idiom ChannelPolicyFields uses).
  setDraft: (next: FillerDraft) => void;
  // Three-state break frequency: undefined follows the global default, 0 disables breaks,
  // and a positive number is this channel's own frequency.
  breaksPerHour: number | undefined;
  setBreaksPerHour: (next: number | undefined) => void;
  // Two-state break length: undefined follows the global default; a duration string is
  // this channel's override. Turning breaks off remains the frequency control's job.
  breakDuration: string | undefined;
  setBreakDuration: (next: string | undefined) => void;
  // The assembled break for the CURRENT draft — the same PodPoolDTO shape the
  // saved GET returns, so it drops straight into PodTimeline. Undefined until the first
  // preview lands.
  preview?: PodPoolDTO;
  // A preview request is in flight (initial load or a debounced re-assemble).
  isPreviewing: boolean;
  // The last preview failed (e.g. a 422 the client-side guard missed) — surfaced inline
  // rather than swallowed, so a broken sandbox never looks like an empty one.
  previewError: unknown;
  // The draft differs from what's saved — drives whether Apply/Discard are offered.
  isDirty: boolean;
  // Persist the filler selection, break frequency and length together — reconcile + SSE take it
  // from there. Applying is what ends the draft session.
  apply: () => void;
  // A save is in flight.
  isApplying: boolean;
  // Throw the draft away, back to saved.
  discard: () => void;
};

export type { ChannelFillerDraft, FillerDraft };
