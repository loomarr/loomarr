// A scriptable EventSource for tests and stories. jsdom has none, and the SSE bus constructs one
// on mount; this one records every stream the app opens and lets a test push a frame down it, so a
// test drives the real subscribe -> dispatch -> fan-out path instead of reading its source.
//
// It speaks only the spec's frame vocabulary (`EVENT_TYPES`): emitting, or listening for, a frame
// the server never sends throws, because a fake that delivers anything hides the typo.
//
// Installed as the global in `setup.ts`, which empties `instances` after each test.
import { EVENT_TYPES, type EventType } from "@loomarr/core/events";

type Listener = (event: MessageEvent<string>) => void;

// The connection events every EventSource fires besides the server's named frames.
const CONNECTION_EVENTS: readonly string[] = ["open", "error", "message"];

const assertFrameType = (type: string): void => {
  if (!(EVENT_TYPES as readonly string[]).includes(type)) {
    throw new Error(`FakeEventSource: "${type}" is not a frame type the spec declares`);
  }
};

class FakeEventSource {
  static instances: FakeEventSource[] = [];

  static reset(): void {
    FakeEventSource.instances = [];
  }

  /** Deliver one frame to every stream the app has open. */
  static broadcast(type: EventType, payload: unknown): void {
    for (const stream of FakeEventSource.instances) {
      if (!stream.closed) stream.emit(type, payload);
    }
  }

  readonly listeners = new Map<string, Set<Listener>>();
  closed = false;

  constructor(
    readonly url: string,
    readonly init?: EventSourceInit,
  ) {
    FakeEventSource.instances.push(this);
  }

  addEventListener(type: string, listener: Listener): void {
    if (!CONNECTION_EVENTS.includes(type)) assertFrameType(type);
    const set = this.listeners.get(type) ?? new Set<Listener>();
    set.add(listener);
    this.listeners.set(type, set);
  }

  removeEventListener(type: string, listener: Listener): void {
    this.listeners.get(type)?.delete(listener);
  }

  close(): void {
    this.closed = true;
  }

  /** The frame types something subscribed to. */
  subscribedTypes(): string[] {
    return [...this.listeners.entries()].filter(([, set]) => set.size > 0).map(([type]) => type);
  }

  /** Deliver one frame, JSON-encoded as the server sends it, to this stream's listeners. */
  emit(type: EventType, payload: unknown): void {
    assertFrameType(type);
    for (const listener of this.listeners.get(type) ?? []) {
      listener(new MessageEvent(type, { data: JSON.stringify(payload) }));
    }
  }
}

export { FakeEventSource };
