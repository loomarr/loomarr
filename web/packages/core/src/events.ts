// The SSE invalidation bus (frontend-design §4.2; main doc §8). It mirrors the
// BE's /v1/events frame vocabulary and turns each frame into TanStack Query cache
// invalidations — the coarse "invalidate-and-refetch" contract, never a surgical
// patch stream: a dropped frame is a latency bug, GET is the source of truth on
// reconnect (§8, finding 5 of the FE↔BE audit). Platform-agnostic: the parsing +
// invalidation logic is shared; only the EventSource construction is web-specific
// (RN injects a polyfill with the same interface).

import { useEffect, useRef } from "react";
import { useQueryClient, type QueryClient } from "@tanstack/react-query";

// Frame payloads, matching the BE bus (internal/app/emitter.go, systemllm.go).
export interface TitleEvent {
  key: string;
  state: string;
  name: string;
}
export interface ChannelEvent {
  id?: string;
  [k: string]: unknown;
}
export type SuggestionPhase = "searching" | "reasoning" | "scoring" | "done" | "failed";
export interface SuggestionEvent {
  jobId: string;
  phase: SuggestionPhase;
}
export interface LlmPullEvent {
  model?: string;
  status?: string;
  completed?: number;
  total?: number;
  [k: string]: unknown;
}

export interface EventHandlers {
  onTitle?: (e: TitleEvent) => void;
  onChannel?: (e: ChannelEvent) => void;
  onSuggestion?: (e: SuggestionEvent) => void;
  onLlmPull?: (e: LlmPullEvent) => void;
}

export const EVENTS_URL = "/v1/events";

// openEventStream subscribes to the named SSE frames and returns a close fn. Same-
// origin with cookie auth (withCredentials). Malformed frames are ignored (latency
// bus, never load-bearing).
export function openEventStream(handlers: EventHandlers, url: string = EVENTS_URL): () => void {
  const es = new EventSource(url, { withCredentials: true });
  const on = <T>(type: string, cb?: (e: T) => void) => {
    if (!cb) return;
    es.addEventListener(type, (ev) => {
      try {
        cb(JSON.parse((ev as MessageEvent).data) as T);
      } catch {
        /* ignore malformed frame */
      }
    });
  };
  on<TitleEvent>("title", handlers.onTitle);
  on<ChannelEvent>("channel", handlers.onChannel);
  on<SuggestionEvent>("suggestion", handlers.onSuggestion);
  on<LlmPullEvent>("llm_pull", handlers.onLlmPull);
  return () => es.close();
}

// invalidateByPrefix invalidates every query whose first key element (the URL, per
// orval's fetch client) starts with prefix — coarse but correct.
export function invalidateByPrefix(qc: QueryClient, prefix: string): void {
  void qc.invalidateQueries({
    predicate: (q) => typeof q.queryKey[0] === "string" && (q.queryKey[0] as string).startsWith(prefix),
  });
}

// useLoomarrEvents opens the stream for the app's lifetime and wires the standard
// invalidations; `extra` handlers (e.g. a workspace consuming suggestion phases)
// run after invalidation and are read through a ref so updating them never
// reconnects the stream.
export function useLoomarrEvents(extra?: EventHandlers): void {
  const qc = useQueryClient();
  const extraRef = useRef(extra);
  extraRef.current = extra;

  useEffect(() => {
    return openEventStream({
      onTitle: (e) => {
        invalidateByPrefix(qc, "/v1/titles");
        invalidateByPrefix(qc, "/v1/suggestions");
        extraRef.current?.onTitle?.(e);
      },
      onChannel: (e) => {
        invalidateByPrefix(qc, "/v1/channels");
        extraRef.current?.onChannel?.(e);
      },
      onSuggestion: (e) => {
        invalidateByPrefix(qc, "/v1/suggestions");
        extraRef.current?.onSuggestion?.(e);
      },
      onLlmPull: (e) => {
        invalidateByPrefix(qc, "/v1/system/llm");
        extraRef.current?.onLlmPull?.(e);
      },
    });
  }, [qc]);
}
