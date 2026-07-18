// Frame payloads, matching the BE bus (internal/app/emitter.go, systemllm.go).
interface TitleEvent {
  key: string;
  state: string;
  name: string;
}

interface ChannelEvent {
  id?: string;
  [k: string]: unknown;
}

type SuggestionPhase = "searching" | "reasoning" | "scoring" | "done" | "failed";

interface SuggestionEvent {
  jobId: string;
  phase: SuggestionPhase;
}

interface LlmPullEvent {
  model?: string;
  status?: string;
  completed?: number;
  total?: number;
  [k: string]: unknown;
}

interface EventHandlers {
  onTitle?: (e: TitleEvent) => void;
  onChannel?: (e: ChannelEvent) => void;
  onSuggestion?: (e: SuggestionEvent) => void;
  onLlmPull?: (e: LlmPullEvent) => void;
}

export type { ChannelEvent, EventHandlers, LlmPullEvent, SuggestionEvent, SuggestionPhase, TitleEvent };
