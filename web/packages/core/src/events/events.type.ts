// Frame payloads for the /v1/events SSE stream.
//
// ⚠ **GENERATED, not hand-mirrored.** The Go frames are typed DTOs (internal/api/events.go),
// they reach api/openapi.yaml through huma's sse.Register, and orval generates these. Do not
// hand-write frame interfaces here: one shape defined twice, in two languages, with nothing
// binding them drifts silently — a misspelled field (`id` for `channelId`) reads `undefined`
// forever behind an `[k: string]: unknown` index signature, and a missing one (`percent`) gets
// recomputed worse in the UI. `round` is a number and `channelId` is spelled right because the
// spec says so, not because someone remembered.
//
// Re-exported (rather than imported directly at each call site) so the SSE vocabulary still
// reads as one list, and so a frame added on the backend shows up here as a compile error
// rather than as a listener that never fires.

import type { ActivityEvent } from "@loomarr/api/models/activityEvent";
import type { ChannelEvent } from "@loomarr/api/models/channelEvent";
import type { DatabaseEvent } from "@loomarr/api/models/databaseEvent";
import type { Events200Item } from "@loomarr/api/models/events200Item";
import type { FillerClipEvent } from "@loomarr/api/models/fillerClipEvent";
import type { FillerIngestEvent } from "@loomarr/api/models/fillerIngestEvent";
import type { FillerSplitEvent } from "@loomarr/api/models/fillerSplitEvent";
import type { HealthEvent } from "@loomarr/api/models/healthEvent";
import type { JobEvent } from "@loomarr/api/models/jobEvent";
import type { LibraryScanEvent } from "@loomarr/api/models/libraryScanEvent";
import type { LibraryTitlesEvent } from "@loomarr/api/models/libraryTitlesEvent";
import type { LLMPullEvent } from "@loomarr/api/models/lLMPullEvent";
import type { PlayoutEvent } from "@loomarr/api/models/playoutEvent";
import type { SuggestionEvent } from "@loomarr/api/models/suggestionEvent";
import type { SuggestionEventPhase } from "@loomarr/api/models/suggestionEventPhase";
import type { TitleEvent } from "@loomarr/api/models/titleEvent";

// Kept under the frontend's historical names so call sites don't churn: the Go type is
// LLMPullEvent (Go initialisms), the frontend has always called it LlmPullEvent, and
// SuggestionPhase reads better than SuggestionEventPhase at a use site.
type LlmPullEvent = LLMPullEvent;
type SuggestionPhase = SuggestionEventPhase;

// The frame vocabulary is the generated `GET /v1/events` item union, so a frame added to the spec
// is a new member of every type below and a compile error wherever a per-frame table lacks it.
/** The SSE event names the spec declares: "title", "channel", "filler_clip", … */
type EventType = Events200Item["event"];

/** Each event name's payload type. */
type EventPayloads = { [E in Events200Item as E["event"]]: E["data"] };

type Pascal<S extends string> = S extends `${infer Head}_${infer Rest}`
  ? `${Capitalize<Head>}${Pascal<Rest>}`
  : Capitalize<S>;

/** The handler name a frame type maps to: "filler_clip" is "onFillerClip". */
type EventHandlerName<T extends EventType> = `on${Pascal<T>}`;

// Every key is optional because a consumer subscribes to the frames it cares about. The key set is
// derived from the spec, so no hand-written list of handler names exists to fall out of step.
type EventHandlers = { [T in EventType as EventHandlerName<T>]?: (e: EventPayloads[T]) => void };

/** The one dispatcher the stream calls for every frame, whatever its type. */
type FrameHandler = <T extends EventType>(type: T, payload: EventPayloads[T]) => void;

interface EventStreamMessage {
  data: string;
}

interface EventStreamPort {
  addEventListener: (type: string, listener: (event: EventStreamMessage) => void) => void;
  close: () => void;
}

type EventStreamFactory = (url: string) => EventStreamPort;

export type {
  ActivityEvent,
  ChannelEvent,
  DatabaseEvent,
  EventHandlerName,
  EventHandlers,
  EventPayloads,
  EventStreamFactory,
  EventStreamMessage,
  EventStreamPort,
  EventType,
  FillerClipEvent,
  FillerIngestEvent,
  FillerSplitEvent,
  FrameHandler,
  HealthEvent,
  JobEvent,
  LibraryScanEvent,
  LibraryTitlesEvent,
  LlmPullEvent,
  PlayoutEvent,
  SuggestionEvent,
  SuggestionPhase,
  TitleEvent,
};
