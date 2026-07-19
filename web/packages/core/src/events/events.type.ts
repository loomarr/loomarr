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

// Mirrors the BE's llm_pull frame (internal/app/systemllm.go publishPull). Every field
// it actually sends is declared, because the index signature below otherwise hides a
// missing one: `percent` was absent here while the BE had been sending it all along, so
// the UI recomputed a worse version and showed nothing during "starting".
interface LlmPullEvent {
  jobId?: string;
  model?: string;
  // Ollama's own status strings pass through, plus Loomarr's terminal "success"/"error".
  status?: string;
  // BE-computed, 0-100. It is -1 on failure — a sentinel, never a percentage to render.
  percent?: number;
  completed?: number;
  total?: number;
  // Set when status is "error"; the reason the download failed.
  error?: string;
  [k: string]: unknown;
}

// Mirrors the BE's filler_ingest frame (internal/app/filler.go publishIngest). Every
// field it sends is declared for the same reason LlmPullEvent's are: the index signature
// would otherwise hide a missing one, and the UI would silently render a worse version.
interface FillerIngestEvent {
  jobId?: string;
  // "starting" | "success" | "error".
  status?: string;
  fetched?: number;
  skipped?: number;
  failed?: number;
  // Sources that returned no clips AND no error — almost always a typo'd Archive id,
  // which returns 200 with nothing. Surfaced so "fetched: 0" has a reason attached.
  empty?: number;
  error?: string;
  [k: string]: unknown;
}

interface EventHandlers {
  onTitle?: (e: TitleEvent) => void;
  onChannel?: (e: ChannelEvent) => void;
  onSuggestion?: (e: SuggestionEvent) => void;
  onLlmPull?: (e: LlmPullEvent) => void;
  onFillerIngest?: (e: FillerIngestEvent) => void;
}

export type {
  ChannelEvent,
  EventHandlers,
  FillerIngestEvent,
  LlmPullEvent,
  SuggestionEvent,
  SuggestionPhase,
  TitleEvent,
};
