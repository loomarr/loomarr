import { describe, expect, it } from "vitest";
import { clipMediaURL } from "./clip-thumb";

// §10 V45a: a clip's wire identity is its content HASH — hex, no slashes, no special
// characters — so the route is a plain `/v1/filler/media/{hash}` with a single
// `encodeURIComponent` over the WHOLE value. That replaced the old path-based per-segment
// encoding (preserving `/` while escaping `?`/`#`/non-ASCII) — a hash has no segments to
// preserve, so there is no reason to treat it any differently from an ordinary path param.
//
// ⚠ **`clipThumbURL`/`clipHoverURL` were retired in V52 phase 8** with the routes they addressed;
// a clip's still and hover loop are image-service images now (§22). Their assertions are not
// simply deleted — the encoding rule and the pass-through rule were never per-route properties,
// so they are asserted here against the one builder that remains.

describe("clipMediaURL", () => {
  it("builds the route path for a hash", () => {
    expect(clipMediaURL("a3f9e2")).toBe("/v1/filler/media/a3f9e2");
  });

  // Not a real hash shape, but nothing stops a stray value from carrying one of these — the
  // encoding must still make a safe URL out of it rather than assume hex input.
  it("encodes characters that would otherwise be unsafe in a URL segment", () => {
    expect(clipMediaURL("a b?c#d")).toBe("/v1/filler/media/a%20b%3Fc%23d");
  });

  it("encodes non-ASCII input", () => {
    expect(clipMediaURL("café")).toBe("/v1/filler/media/caf%C3%A9");
  });

  // ⚠ Unlike the old path-based route, a slash is now just another character to escape — a
  // hash has none, so there is no separator to preserve and %2F is the correct, safe encoding.
  // It also matters for identity: a clip whose hash carried an unescaped separator would play a
  // different file than the one whose card was clicked.
  it("escapes a slash rather than preserving it as a path separator", () => {
    expect(clipMediaURL("a/b")).toBe("/v1/filler/media/a%2Fb");
  });

  // ⚠ **`data:` passes through, `http(s):` must NOT.** The pass-through is what lets stories
  // render offline (the visual suite runs against storybook-static with no server); allowing any
  // absolute URL would let a clip's identity become a resource fetched from an arbitrary origin —
  // a beacon that leaks who is browsing the catalog and when. §22 generalised that reasoning to
  // every image in the product; it still binds here, for the one asset that is not an image.
  it("passes a data URI through but never an absolute http URL", () => {
    const inline = "data:video/webm;base64,AAAA";
    expect(clipMediaURL(inline)).toBe(inline);

    // ⚠ THE assertion: an absolute URL is treated as an ordinary value and encoded whole, never
    // returned as-is. It comes back as nonsense that 404s, which is the correct outcome — the
    // point is only that the browser never fetches `evil.example`.
    const remote = "https://evil.example/track.webm";
    expect(clipMediaURL(remote)).toBe("/v1/filler/media/https%3A%2F%2Fevil.example%2Ftrack.webm");
    expect(clipMediaURL(remote)).not.toBe(remote);
    expect(clipMediaURL(remote).startsWith("/v1/filler/media/")).toBe(true);
  });
});
