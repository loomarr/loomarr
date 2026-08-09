import { describe, expect, it } from "vitest";
import { clipHoverURL, clipMediaURL, clipThumbURL } from "./clip-thumb";

// §10 V45a: a clip's wire identity is its content HASH — hex, no slashes, no special
// characters — so these routes are a plain `/v1/filler/{route}/{hash}` with a single
// `encodeURIComponent` over the WHOLE value. That replaces the old path-based per-segment
// encoding (preserving `/` while escaping `?`/`#`/non-ASCII) — a hash has no segments to
// preserve, so there is no reason to treat it any differently from an ordinary path param.

describe("clipThumbURL", () => {
  it("builds the route path for a hash", () => {
    expect(clipThumbURL("a3f9e2")).toBe("/v1/filler/thumb/a3f9e2");
  });

  // Not a real hash shape, but nothing stops a stray value from carrying one of these — the
  // encoding must still make a safe URL out of it rather than assume hex input.
  it("encodes characters that would otherwise be unsafe in a URL segment", () => {
    expect(clipThumbURL("a b?c#d")).toBe("/v1/filler/thumb/a%20b%3Fc%23d");
  });

  it("encodes non-ASCII input", () => {
    expect(clipThumbURL("café")).toBe("/v1/filler/thumb/caf%C3%A9");
  });

  // ⚠ Unlike the old path-based route, a slash is now just another character to escape — a
  // hash has none, so there is no separator to preserve and %2F is the correct, safe encoding.
  it("escapes a slash rather than preserving it as a path separator", () => {
    expect(clipThumbURL("a/b")).toBe("/v1/filler/thumb/a%2Fb");
  });
});

// V39 added two more routes over the same encoding rule. These assert the parts that are NOT
// shared — the route each one targets — plus the `data:` pass-through, which is load-bearing for
// both Storybook and for not turning a clip's identity into a request to an arbitrary origin.
describe("clipHoverURL / clipMediaURL", () => {
  it("targets its own route", () => {
    // ⚠ `hover`, not `preview`. /channels/{id}/filler/preview is a different thing entirely (the
    // pod pool a channel would get, as JSON), which is why the server refused the name.
    expect(clipHoverURL("a3f9e2")).toBe("/v1/filler/hover/a3f9e2");
    expect(clipMediaURL("a3f9e2")).toBe("/v1/filler/media/a3f9e2");
  });

  // The shared encoding rule reaches both, or a clip identified by a hash containing an
  // unsafe character would play a different file than the one whose card was clicked.
  it("encodes segments the same way the thumbnail route does", () => {
    expect(clipHoverURL("a b?c")).toBe("/v1/filler/hover/a%20b%3Fc");
    expect(clipMediaURL("a#b")).toBe("/v1/filler/media/a%23b");
  });

  // ⚠ **`data:` passes through, `http(s):` must NOT.** The pass-through is what lets stories
  // render offline (the visual suite runs against storybook-static with no server); allowing any
  // absolute URL would let a clip's identity become a resource fetched from an arbitrary origin —
  // a beacon that leaks who is browsing the catalog and when.
  it("passes a data URI through but never an absolute http URL", () => {
    const inline = "data:video/webm;base64,AAAA";
    expect(clipHoverURL(inline)).toBe(inline);
    expect(clipMediaURL(inline)).toBe(inline);

    // ⚠ THE assertion: an absolute URL is treated as an ordinary value and encoded whole, never
    // returned as-is. It comes back as nonsense that 404s, which is the correct outcome — the
    // point is only that the browser never fetches `evil.example`.
    const remote = "https://evil.example/track.webp";
    expect(clipHoverURL(remote)).toBe("/v1/filler/hover/https%3A%2F%2Fevil.example%2Ftrack.webp");
    expect(clipHoverURL(remote)).not.toBe(remote);
    expect(clipHoverURL(remote).startsWith("/v1/filler/hover/")).toBe(true);
  });
});
