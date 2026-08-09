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
// by this provider. The ingest panel was the frame's only consumer (retired-ok, V38b) and its
// callback could never fire, so "Download clips" sat at "starting" forever. A whole feature
// dead, with a green build, green types and green tests. A doc-drift audit found it by reading,
// not by running.
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

// ⚠ **There are TWO handler objects in that file, and BOTH have to be complete.** A frame reaches
// a component through both in series: `fanOut` (what the provider subscribes to the stream) and
// the re-dispatch inside `useLoomarrEventListener` (what forwards to each listener's callbacks). A
// key missing from either end kills the frame just as dead.
//
// ⚠ This guard used to regex the WHOLE FILE in one pass and compare core against the UNION of the
// two objects — so a key present in only one satisfied it. It was green while
// `useLoomarrEventListener` was missing `onPlayout` and `onDatabase`, which meant
// `settings/system/database.tsx`'s `onDatabase` subscription could never fire and the migration
// page's live progress was dead. A guard against drift that ORs its two sources cannot see drift
// between them.
//
// So: slice first, assert each half separately.
const providerSections = (): { fanOut: string; listener: string } => {
  const src = readFileSync(PROVIDER, "utf8");
  const fanOutAt = src.indexOf("const fanOut = useMemo<EventHandlers>(");
  const listenerAt = src.indexOf("return subscribe({");
  // ⚠ Throw rather than fall back. If either anchor is renamed, silently returning the whole file
  // (or an empty slice) makes this test pass vacuously — which is the precise failure mode it was
  // just fixed for.
  if (fanOutAt < 0 || listenerAt < 0 || listenerAt <= fanOutAt) {
    throw new Error(
      "events-provider.tsx no longer contains the two handler objects this guard parses — " +
        "update the anchors, do not delete the check",
    );
  }
  return { fanOut: src.slice(fanOutAt, listenerAt), listener: src.slice(listenerAt) };
};

const handlerNamesIn = (src: string): string[] =>
  [...new Set([...src.matchAll(/^\s+(on[A-Z][A-Za-z]*):/gm)].map((m) => m[1] ?? ""))].filter(Boolean).sort();

describe("LoomarrEventsProvider", () => {
  it("reads a non-empty handler list from every source", () => {
    // Guards the regexes themselves: if any stops matching, the comparisons below would
    // pass vacuously and this test would silently stop watching anything.
    const { fanOut, listener } = providerSections();
    expect(coreHandlerNames().length).toBeGreaterThanOrEqual(5);
    expect(handlerNamesIn(fanOut).length).toBeGreaterThanOrEqual(5);
    expect(handlerNamesIn(listener).length).toBeGreaterThanOrEqual(5);
  });

  it.each([
    ["fanOut", (s: ReturnType<typeof providerSections>) => s.fanOut],
    ["useLoomarrEventListener", (s: ReturnType<typeof providerSections>) => s.listener],
  ])("%s forwards every frame the core hook subscribes to", (which, pick) => {
    const names = handlerNamesIn(pick(providerSections()));
    const missing = coreHandlerNames().filter((n) => !names.includes(n));
    expect(
      missing,
      `${which} does not forward ${missing.join(", ")} — any component listening for that ` +
        `frame will never be called, and TypeScript cannot tell you because every ` +
        `EventHandlers key is optional`,
    ).toEqual([]);
  });
});
