import type { ChannelPolicy, PreviewProgrammingOutputBody } from "@loomarr/api";

interface ChannelRulesDraft {
  // The policy being edited. Seeded from the channel's saved policy, resynced when THAT
  // changes (an apply landing, a refine reseeding rules) — never on an incidental re-render.
  draft: ChannelPolicy;
  setDraft: (next: ChannelPolicy) => void;
  // What would air if the draft were applied, resolved through the same ComputeDesiredAt the
  // reconciler runs. Undefined until the first preview lands.
  preview?: PreviewProgrammingOutputBody;
  isPreviewing: boolean;
  previewError: unknown;
  // The wall-clock the preview is evaluated at ("" = now). Lives here rather than in the
  // panel because it is a preview INPUT — changing it must re-POST, so it shares the
  // debounce with the draft rather than racing it.
  at: string;
  setAt: (next: string) => void;
  // False when the draft is byte-identical to saved — which is what gates the Apply bar. A
  // dirty check on the object reference would be true on every render.
  isDirty: boolean;
  apply: () => void;
  isApplying: boolean;
  discard: () => void;
}

export type { ChannelRulesDraft };
