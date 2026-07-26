import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

// THE PROVIDER MUST FAN OUT EVERY FRAME THE CORE HOOK SUBSCRIBES TO.
//
// This exists because the omission is invisible to TypeScript. `EventHandlers` marks every
// handler optional — correctly, since a consumer subscribes to the one frame it cares about —
// so a provider that implements four of six type-checks perfectly.
//
// That is exactly what happened: `onFillerIngest` was subscribed by the core hook and dropped
// by this provider. IngestPanel is the frame's only consumer, and its callback could never
// fire, so "Download clips" sat at "starting" forever. A whole feature dead, with a green
// build, green types and green tests. A doc-drift audit found it by reading, not by running.
//
// Parsing the SOURCE rather than importing and probing is deliberate: the handlers are wired
// inside a `useMemo` in a React component, so there is nothing to enumerate at runtime without
// rendering, and rendering would only prove the keys that were remembered.
const HERE = dirname(fileURLToPath(import.meta.url));
const CORE_EVENTS = join(
  HERE,
  "..",
  "..",
  "..",
  "..",
  "..",
  "packages",
  "core",
  "src",
  "events",
  "events.ts",
);
const PROVIDER = join(HERE, "events-provider.tsx");

// Every `handlers.onX` the core stream reads — the authoritative list, because that file is
// what decides which SSE frame names are subscribed at all.
const coreHandlerNames = (): string[] => {
  const src = readFileSync(CORE_EVENTS, "utf8");
  return [...new Set([...src.matchAll(/handlers\.(on[A-Z][A-Za-z]*)/g)].map((m) => m[1] ?? ""))]
    .filter(Boolean)
    .sort();
};

// Every `onX:` the provider defines in its fan-out object.
const providerHandlerNames = (): string[] => {
  const src = readFileSync(PROVIDER, "utf8");
  return [...new Set([...src.matchAll(/^\s{6}(on[A-Z][A-Za-z]*):/gm)].map((m) => m[1] ?? ""))]
    .filter(Boolean)
    .sort();
};

describe("LoomarrEventsProvider", () => {
  it("reads a non-empty handler list from core", () => {
    // Guards the regexes themselves: if either stops matching, the comparison below would
    // pass vacuously and this test would silently stop watching anything.
    expect(coreHandlerNames().length).toBeGreaterThanOrEqual(5);
    expect(providerHandlerNames().length).toBeGreaterThanOrEqual(5);
  });

  it("fans out every frame the core hook subscribes to", () => {
    const missing = coreHandlerNames().filter((n) => !providerHandlerNames().includes(n));
    expect(
      missing,
      `the provider does not fan out ${missing.join(", ")} — any component listening for ` +
        `that frame will never be called, and TypeScript cannot tell you because every ` +
        `EventHandlers key is optional`,
    ).toEqual([]);
  });
});
