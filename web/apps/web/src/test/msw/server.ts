import { setupServer } from "msw/node";

// The shared MSW server — the frontend's answer to `internal/testkit` (AGENTS.md: "Unit tests never
// invent private mocks; extend the testkit"). Before V53d, 31 test files each hand-rolled a local
// `stubFetch`, so 31 places independently encoded what the wire looks like, and each was free to
// drift from it silently. That is the same class as `maxAcquire`/`maxAcquisitions` and the
// `seasons`-omitempty bug: a value that serializes fine and the server ignores.
//
// Handlers come from orval's generated `get*MockHandler(override)` helpers so the URL, method and
// status are DERIVED. When a route is renamed — `/v1/suggestions` → `/v1/proposals` (retired-ok) really
// happened here — a regenerate fixes every handler, where a hand-written string would silently
// stop matching and the test would keep passing against nothing.
//
// ⚠ The generated DATA is never trusted. Tests pass a typed fixture; see `../fixtures`.
const server = setupServer();

// Every request MSW did not handle, recorded for the afterEach assertion below.
const unhandled: string[] = [];

// ⚠ `onUnhandledRequest: "error"` DOES NOT FAIL THE TEST, and that is the trap this module exists
// to avoid. MSW's own docs define it as "print an error and halt request execution" — the request
// rejects and the error is logged, but the test runner never sees an exception, so a spec with no
// assertion on the result passes anyway. The maintainer confirmed the mechanism in mswjs/msw#946:
// the interceptor handles the exception exactly as the native class would (by aborting the
// request), so "from MSW's perspective no exception has happened".
//
// Recording and asserting is what actually turns an unmatched request red — which is the guard
// that catches a renamed route, a typo'd path, or a component that started fetching something the
// test never modelled.
const installServerLifecycle = (hooks: {
  beforeAll: (fn: () => void) => void;
  afterEach: (fn: () => void) => void;
  afterAll: (fn: () => void) => void;
}): void => {
  hooks.beforeAll(() => {
    server.listen({
      onUnhandledRequest: (request, print) => {
        unhandled.push(`${request.method} ${request.url}`);
        print.error(); // keep MSW's verbose message — it names the request and suggests a handler
      },
    });
  });

  hooks.afterEach(() => {
    // Snapshot and clear BEFORE asserting, so one unhandled request fails exactly the test that
    // caused it instead of every test that follows it.
    const seen = unhandled.splice(0, unhandled.length);
    server.resetHandlers();
    if (seen.length > 0) {
      throw new Error(
        `MSW had no handler for:\n  ${seen.join("\n  ")}\n` +
          "Add a handler (see src/test/msw) or, if the route moved, regenerate the client.",
      );
    }
  });

  hooks.afterAll(() => server.close());
};

export { installServerLifecycle, server };
