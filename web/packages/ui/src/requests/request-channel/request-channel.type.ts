import type { ChannelIdeaDTO, Intent, RequestsRole, RequestsStatus } from "@loomarr/core/requests";

/** Where the typed brief stands. `unavailable` keeps the draft: only an administrator can fix it. */
type BriefState =
  | { kind: "failed"; message: string }
  | { kind: "idle" }
  | { kind: "sending" }
  | { kind: "unavailable"; reason: "ai" | "grounding" };

type RequestChannelProps = {
  brief: BriefState;
  /** The idea just hidden, so its Undo can be offered. */
  hiddenIdea?: ChannelIdeaDTO;
  ideas: readonly ChannelIdeaDTO[];
  ideasStatus: RequestsStatus;
  /** Resumes a request that couldn't be built: its text and constraints come back to the form. */
  initialIntent?: Intent;
  nowMs: number;
  onBack: () => void;
  onHideIdea: (ideaId: string) => void;
  onRequestIdea: (ideaId: string) => void;
  onRetryIdeas: () => void;
  onSubmitBrief: (intent: Intent) => void;
  onUndoHide: () => void;
  /** The idea whose request is in flight. */
  requestingIdeaId?: string;
  /** The role from `/v1/auth/me`; it only changes whose job a setup refusal is. */
  viewer: RequestsRole;
};

export type { BriefState, RequestChannelProps };
