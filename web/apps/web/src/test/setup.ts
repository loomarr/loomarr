// Vitest global setup — jest-dom matchers (toBeInTheDocument, toHaveAccessibleName,
// …) for the Testing Library component tests. Loaded via vite.config test.setupFiles.
import { configure } from "@testing-library/react";
import { afterAll, afterEach, beforeAll } from "vitest";
import { installServerLifecycle } from "./msw/server";
import "@testing-library/jest-dom/vitest";

// Testing Library's async utilities (`findBy*`, `waitFor`) default to a 1000ms timeout, which is
// generous for one spec and tight for 156 running in parallel on a loaded machine.
//
// ⚠ This is a LOAD tolerance, not a correctness knob. It was raised in V50b because the Base UI
// migration turned a batch of `getBy` queries into `findBy` — popups now mount asynchronously —
// and the suite went from 2 intermittent failures to 6, every one of them passing in isolation.
// Waiting longer cannot make a broken assertion pass: a query that never resolves still fails,
// just later. What it removes is a red build caused by CPU contention rather than by the code.
configure({ asyncUtilTimeout: 5000 });

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

// The shared MSW server (V53d) — one mock layer for the whole suite, replacing the per-file
// `stubFetch` helpers that 31 test files each hand-rolled. See `./msw/server` for why the
// unhandled-request guard records-and-asserts instead of using `onUnhandledRequest: "error"`,
// which does not fail a test.
//
// ⚠ Installed globally but STARTS WITH NO HANDLERS, so a test that has not been migrated is
// unaffected: its `vi.stubGlobal("fetch", …)` replaces global fetch outright and never reaches
// MSW's interceptor. That is what makes the migration incremental rather than a 31-file
// big-bang — the two mechanisms coexist until the last stub is gone.
installServerLifecycle({ beforeAll, afterEach, afterAll });
