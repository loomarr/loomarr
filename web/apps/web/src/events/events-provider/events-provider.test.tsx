import {
  EVENT_INVALIDATIONS,
  EVENT_TYPES,
  type EventHandlers,
  type EventType,
  eventHandlerName,
} from "@loomarr/core/events";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { FakeEventSource } from "@/test/fake-event-source";
import { LoomarrEventsProvider, useLoomarrEventListener } from "./events-provider";

// THE STREAM MUST DELIVER EVERY FRAME TYPE THE SPEC DECLARES.
//
// The omission this guards is invisible to TypeScript at the handler level: every `EventHandlers`
// key is optional, so a hand-written subscribe list that forgets one frame type-checks. That is how
// `database` went dead (no listener ever fired, so the migration page's progress never moved) and
// how `filler_ingest` was dropped before it. The type list now comes from the generated spec, so
// this test drives the real stream — a fake EventSource emitting one frame of EVERY type — rather
// than reading source, and fails if any type reaches no listener or invalidates the wrong queries.

// Payloads for the frames whose invalidation is conditional on the payload; every other frame
// invalidates unconditionally, so any object serves.
const PAYLOADS: Partial<Record<EventType, unknown>> = {
  filler_ingest: { status: "success" },
  filler_clip: { disposition: "filed" },
  library_scan: { step: "idle" },
};
const payloadFor = (type: EventType): unknown => PAYLOADS[type] ?? { frame: type };

const Listener = ({ handlers }: { handlers: EventHandlers }) => {
  useLoomarrEventListener(handlers);
  return null;
};

const mount = (children: ReactNode) => {
  const queryClient = new QueryClient();
  render(
    <QueryClientProvider client={queryClient}>
      <LoomarrEventsProvider>{children}</LoomarrEventsProvider>
    </QueryClientProvider>,
  );
  const stream = FakeEventSource.instances[0];
  if (!stream) throw new Error("the provider did not open an event stream");
  return { queryClient, stream };
};

describe("LoomarrEventsProvider", () => {
  it("opens one stream and subscribes to every frame type the spec declares", () => {
    const { stream } = mount(null);
    expect(FakeEventSource.instances).toHaveLength(1);
    // The type list and the invalidation table are one source: nothing can be listed in one
    // and left out of the other.
    expect(EVENT_TYPES).toEqual(Object.keys(EVENT_INVALIDATIONS));
    expect(EVENT_TYPES).toContain("database");
    expect(stream.subscribedTypes().sort()).toEqual([...EVENT_TYPES].sort());
  });

  it.each(EVENT_TYPES)("fires a component's handler for a %s frame", (type) => {
    const handler = vi.fn();
    const handlers: EventHandlers = { [eventHandlerName(type)]: handler };
    const { stream } = mount(<Listener handlers={handlers} />);

    stream.emit(type, payloadFor(type));

    expect(handler).toHaveBeenCalledExactlyOnceWith(payloadFor(type));
  });

  it("delivers a frame to every mounted listener and only the matching handler", () => {
    const first = vi.fn();
    const second = vi.fn();
    const other = vi.fn();
    const { stream } = mount(
      <>
        <Listener handlers={{ onDatabase: first }} />
        <Listener handlers={{ onDatabase: second, onJob: other }} />
      </>,
    );

    stream.emit("database", { frame: "database" });

    expect(first).toHaveBeenCalledOnce();
    expect(second).toHaveBeenCalledOnce();
    expect(other).not.toHaveBeenCalled();
  });

  it("stops delivering to a listener once it unmounts", () => {
    const handler = vi.fn();
    const queryClient = new QueryClient();
    const tree = (withListener: boolean) => (
      <QueryClientProvider client={queryClient}>
        <LoomarrEventsProvider>
          {withListener && <Listener handlers={{ onJob: handler }} />}
        </LoomarrEventsProvider>
      </QueryClientProvider>
    );
    const view = render(tree(true));
    const stream = FakeEventSource.instances[0];
    view.rerender(tree(false));

    stream?.emit("job", { frame: "job" });

    expect(handler).not.toHaveBeenCalled();
  });

  it.each(EVENT_TYPES)("invalidates the %s table prefixes and nothing else", async (type) => {
    const { queryClient, stream } = mount(null);
    const { prefixes } = EVENT_INVALIDATIONS[type];
    const unrelated = ["/v1/unrelated"] as const;
    for (const prefix of prefixes) queryClient.setQueryData([`${prefix}/seed`], "seed");
    queryClient.setQueryData([...unrelated], "seed");

    stream.emit(type, payloadFor(type));

    for (const prefix of prefixes) {
      expect(queryClient.getQueryState([`${prefix}/seed`])?.isInvalidated, `${type} → ${prefix}`).toBe(true);
    }
    expect(queryClient.getQueryState([...unrelated])?.isInvalidated).toBe(false);
  });

  it("invalidates the channel and guide prefixes on a channel frame", () => {
    const { queryClient, stream } = mount(null);
    // A prefetched neighbour window and a per-channel read both live under these prefixes.
    queryClient.setQueryData(["/v1/guide", { from: 1, to: 2 }], "guide");
    queryClient.setQueryData(["/v1/guide", { from: 3, to: 4 }], "neighbour");
    for (const suffix of ["now-next", "c1", "c1/pods", "c1/upcoming", "c1/cycle"]) {
      queryClient.setQueryData([`/v1/channels/${suffix}`], suffix);
    }

    stream.emit("channel", { channelId: "c1" });

    for (const q of queryClient.getQueryCache().getAll()) {
      expect(q.state.isInvalidated, JSON.stringify(q.queryKey)).toBe(true);
    }
  });

  it("invalidates the LLM prefix on an llm_pull frame", () => {
    const { queryClient, stream } = mount(null);
    queryClient.setQueryData(["/v1/system/llm/models"], "models");

    stream.emit("llm_pull", { jobId: "j" });

    expect(queryClient.getQueryState(["/v1/system/llm/models"])?.isInvalidated).toBe(true);
  });

  it("holds back the conditional frames until their terminal state", () => {
    const { queryClient, stream } = mount(null);
    queryClient.setQueryData(["/v1/filler/incoming"], "incoming");

    stream.emit("filler_ingest", { status: "running" });
    stream.emit("filler_clip", { disposition: "running" });
    expect(queryClient.getQueryState(["/v1/filler/incoming"])?.isInvalidated).toBe(false);

    stream.emit("filler_clip", { disposition: "filed" });
    expect(queryClient.getQueryState(["/v1/filler/incoming"])?.isInvalidated).toBe(true);
  });
});
