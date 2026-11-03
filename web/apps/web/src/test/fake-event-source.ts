// A scriptable EventSource for tests. jsdom has none, and the SSE bus constructs one on mount;
// this one records every stream the app opens and lets a test push a frame down it, so a test
// drives the real subscribe -> dispatch -> fan-out path instead of reading its source.
//
// Installed as the global in `setup.ts`, which empties `instances` after each test.

type Listener = (event: MessageEvent<string>) => void;

class FakeEventSource {
  static instances: FakeEventSource[] = [];

  static reset(): void {
    FakeEventSource.instances = [];
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
  emit(type: string, payload: unknown): void {
    for (const listener of this.listeners.get(type) ?? []) {
      listener(new MessageEvent(type, { data: JSON.stringify(payload) }));
    }
  }
}

export { FakeEventSource };
