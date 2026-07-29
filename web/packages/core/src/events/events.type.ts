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
  // The 1-based tool-loop round, "0" outside the loop. A STRING because the SSE payload
  // is a flat map of strings on the BE (emitter.go) — declaring it as a number here would
  // typecheck and then compare wrong at runtime.
  //
  // Phases repeat: the model thinks, searches, then thinks again about what came back. The
  // round is what distinguishes "still working, third pass" from "stuck", which is the
  // whole reason a long run needs to look like it is progressing.
  round?: string;
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

// Mirrors the BE's `job` frame (internal/app/emitter.go JobChanged): a scheduled job's
// state changed (started, finished ok/error, or Run-now'd). Carries only the job name — the
// Tasks page refetches GET /v1/jobs on this, keeping the BE the single source of timing truth
// (§18.1).
interface JobEvent {
  name?: string;
  [k: string]: unknown;
}

// Mirrors the BE's `playout` frame (internal/app/app.go, Manager.OnChange): a channel started
// or stopped encoding. Carries only the active count — deliberately a "something changed"
// signal rather than the full telemetry, because the dashboard re-reads
// GET /v1/playout/sessions, which owns the shape.
//
// Fired on session lifecycle only, NOT per ffmpeg progress sample: those arrive about once a
// second per stream, and republishing each would push several frames a second at every open
// browser for numbers that move by fractions.
interface PlayoutEvent {
  active?: number;
  [k: string]: unknown;
}

// Mirrors the BE's `database` frame (internal/app/database.go emit): the SQLite→PostgreSQL
// migration moved (§18, V11). Carries the phase, the parity verdict and per-table counts so
// the stepper's bars can advance without polling.
//
// Like every frame this is a latency optimization — GET /v1/system/database is the truth on
// reconnect — so a dropped frame costs a stale progress bar, never a wrong outcome. That
// matters more here than elsewhere: the operator is watching a data migration, and a UI that
// invented progress it had not been told about would be actively misleading.
interface DatabaseEvent {
  phase?: string;
  parity?: string;
  error?: string;
  [k: string]: unknown;
}

interface EventHandlers {
  onTitle?: (e: TitleEvent) => void;
  onChannel?: (e: ChannelEvent) => void;
  onSuggestion?: (e: SuggestionEvent) => void;
  onLlmPull?: (e: LlmPullEvent) => void;
  onFillerIngest?: (e: FillerIngestEvent) => void;
  onJob?: (e: JobEvent) => void;
  onPlayout?: (e: PlayoutEvent) => void;
  onDatabase?: (e: DatabaseEvent) => void;
}

export type {
  ChannelEvent,
  DatabaseEvent,
  EventHandlers,
  FillerIngestEvent,
  JobEvent,
  LlmPullEvent,
  PlayoutEvent,
  SuggestionEvent,
  SuggestionPhase,
  TitleEvent,
};
