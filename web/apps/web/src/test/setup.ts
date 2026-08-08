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

// DOM APIs jsdom doesn't implement, shimmed so components that use them can render in tests.
//
// ⚠ This block used to read "Radix primitives (Select, …) call DOM APIs jsdom doesn't implement"
// and shimmed FIVE things. Leaving Radix was the moment to check whether that was still true, so
// each shim was removed and the full suite re-run rather than carried forward on faith. The
// results were not what the old comment implied:
//
//   hasPointerCapture / setPointerCapture / releasePointerCapture
//       DELETED. 1209 tests pass without them. They existed for Radix's pointer handling — the
//       old comment's "without these, opening the listbox throws" is no longer true of Base UI.
//
//   scrollIntoView
//       KEPT, but it was never the primitive's need: the caller is OUR OWN `search-command`
//       (`listRef.current?.querySelector(…)?.scrollIntoView`), which keeps the active ⌘K result in
//       view. Removing it fails 31 tests. The old comment credited this to Radix; it was ours.
//
//   ResizeObserver
//       KEPT. The suite passes without it today, but `guide-grid` constructs two of them in app
//       code, so its absence is one un-exercised render away from a confusing throw. `??=` makes
//       keeping it free.
Element.prototype.scrollIntoView ??= () => {};
globalThis.ResizeObserver ??= class {
  observe(): void {}
  unobserve(): void {}
  disconnect(): void {}
} as unknown as typeof ResizeObserver;
