// Vitest global setup — jest-dom matchers (toBeInTheDocument, toHaveAccessibleName,
// …) for the Testing Library component tests. Loaded via vite.config test.setupFiles.
import "@testing-library/jest-dom/vitest";

// jsdom has no EventSource; the SSE bus (core.useLoomarrEvents) constructs one on
// mount. A no-op stub lets components that open the stream render in tests.
class MockEventSource {
  onmessage: ((ev: MessageEvent) => void) | null = null;
  constructor(public url: string) {}
  addEventListener(): void {}
  removeEventListener(): void {}
  close(): void {}
}
globalThis.EventSource = MockEventSource as unknown as typeof EventSource;
