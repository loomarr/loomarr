// Frame payloads for the /v1/events SSE stream.
//
// ⚠ **GENERATED, not hand-mirrored.** These used to be interfaces written here by hand to
// match `map[string]string` literals at ten Go publish sites — one shape defined twice, in two
// languages, with nothing binding them. It drifted exactly as you would expect:
//
//   - `ChannelEvent` declared `id` while the backend has always sent `channelId`, so the field
//     read `undefined` forever. An `[k: string]: unknown` index signature hid it, and the
//     handler only invalidates by prefix, so nothing ever noticed.
//   - `LlmPullEvent.percent` was missing while the backend sent it all along, so the UI
//     recomputed a worse version and showed nothing during "starting".
//   - `SuggestionEvent.round` was a STRING, carrying a comment warning that declaring it a
//     number "would typecheck and then compare wrong at runtime" — a wart of the flat
//     string map, documented instead of fixed.
//
// The Go frames are typed DTOs now (internal/api/events.go), they reach api/openapi.yaml
// through huma's sse.Register, and orval generates these. `round` is a number and `channelId`
// is spelled right because the spec says so, not because someone remembered.
//
// Re-exported (rather than imported directly at each call site) so the SSE vocabulary still
// reads as one list, and so a frame added on the backend shows up here as a compile error
// rather than as a listener that never fires.

import type {
  ActivityEvent,
  ChannelEvent,
  DatabaseEvent,
  FillerClipEvent,
  FillerIngestEvent,
  FillerSplitEvent,
  JobEvent,
  LLMPullEvent,
  PlayoutEvent,
  SuggestionEvent,
  SuggestionEventPhase,
  TitleEvent,
} from "@loomarr/api";

// Kept under the frontend's historical names so call sites don't churn: the Go type is
// LLMPullEvent (Go initialisms), the frontend has always called it LlmPullEvent, and
// SuggestionPhase reads better than SuggestionEventPhase at a use site.
type LlmPullEvent = LLMPullEvent;
type SuggestionPhase = SuggestionEventPhase;

interface EventHandlers {
  onTitle?: (e: TitleEvent) => void;
  onChannel?: (e: ChannelEvent) => void;
  onSuggestion?: (e: SuggestionEvent) => void;
  onLlmPull?: (e: LlmPullEvent) => void;
  onFillerIngest?: (e: FillerIngestEvent) => void;
  onFillerSplit?: (e: FillerSplitEvent) => void;
  onFillerClip?: (e: FillerClipEvent) => void;
  onJob?: (e: JobEvent) => void;
  onPlayout?: (e: PlayoutEvent) => void;
  onDatabase?: (e: DatabaseEvent) => void;
  onActivity?: (e: ActivityEvent) => void;
}

export type {
  ActivityEvent,
  ChannelEvent,
  DatabaseEvent,
  EventHandlers,
  FillerClipEvent,
  FillerIngestEvent,
  FillerSplitEvent,
  JobEvent,
  LlmPullEvent,
  PlayoutEvent,
  SuggestionEvent,
  SuggestionPhase,
  TitleEvent,
};
