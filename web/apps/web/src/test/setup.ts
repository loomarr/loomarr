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

// Radix primitives (Select, …) call DOM APIs jsdom doesn't implement — pointer capture,
// scrollIntoView, ResizeObserver. Shim them so a Select can open and be driven in tests;
// without these, opening the listbox throws "hasPointerCapture is not a function".
Element.prototype.hasPointerCapture ??= () => false;
Element.prototype.setPointerCapture ??= () => {};
Element.prototype.releasePointerCapture ??= () => {};
Element.prototype.scrollIntoView ??= () => {};
globalThis.ResizeObserver ??= class {
  observe(): void {}
  unobserve(): void {}
  disconnect(): void {}
} as unknown as typeof ResizeObserver;
