import { describe, expect, it } from "vitest";
import { clipHoverURL, clipMediaURL, clipThumbURL } from "./clip-thumb";

describe("clipThumbURL", () => {
  it("builds the route path for a nested clip", () => {
    // V28 preserves directory structure rather than flattening, so nested ids are the
    // normal case, not an edge one.
    expect(clipThumbURL("80s/toys/intro.mp4")).toBe("/v1/filler/thumb/80s/toys/intro.mp4");
  });

  // ⚠ Per SEGMENT, never the whole path. encodeURIComponent on the full string escapes the
  // slashes to %2F, and Go's ServeMux will not decode those into separators when matching
  // `{path...}` — the request 404s at the mux, before the handler runs.
  it("keeps the separators unescaped", () => {
    expect(clipThumbURL("a/b/c.mp4")).not.toContain("%2F");
    expect(clipThumbURL("a/b/c.mp4").split("/").length).toBe(7);
  });

  // The characters that make concatenation wrong. `?` starts a query string and `#` a
  // fragment, so an unencoded one truncates the request path silently — the browser asks for
  // a different file and the 404 looks like a missing thumbnail rather than a bug.
  it("encodes characters that would otherwise truncate the URL", () => {
    expect(clipThumbURL("back in 5?.mp4")).toBe("/v1/filler/thumb/back%20in%205%3F.mp4");
    expect(clipThumbURL("a#b.mp4")).toBe("/v1/filler/thumb/a%23b.mp4");
  });

  it("encodes non-ASCII names", () => {
    expect(clipThumbURL("café.mp4")).toBe("/v1/filler/thumb/caf%C3%A9.mp4");
  });

  // ⚠ A `..` segment passes through UNCHANGED, and that is stated here so nobody mistakes
  // this helper for a security boundary. encodeURIComponent leaves dots alone, so the URL can
  // still spell a traversal — it is the server that refuses it, in three places: the browser
  // normalises the path before sending, Go's ServeMux cleans what arrives, and
  // `safeThumbPath` contains the result. A client-side check would be neither necessary nor
  // sufficient, since nothing stops a caller crafting the request by hand.
  it("passes a dot-dot segment through, because containment is the server's job", () => {
    expect(clipThumbURL("../secret.txt")).toBe("/v1/filler/thumb/../secret.txt");
  });
});

// V39 added two more routes over the same encoding rule. These assert the parts that are NOT
// shared — the route each one targets — plus the `data:` pass-through, which is load-bearing for
// both Storybook and for not turning a clip path into a request to an arbitrary origin.
describe("clipHoverURL / clipMediaURL", () => {
  it("targets its own route", () => {
    // ⚠ `hover`, not `preview`. /channels/{id}/filler/preview is a different thing entirely (the
    // pod pool a channel would get, as JSON), which is why the server refused the name.
    expect(clipHoverURL("80s/toys/intro.mp4")).toBe("/v1/filler/hover/80s/toys/intro.mp4");
    expect(clipMediaURL("80s/toys/intro.mp4")).toBe("/v1/filler/media/80s/toys/intro.mp4");
  });

  // The shared encoding rule reaches both, or a clip named `back in 5?.mp4` plays a different
  // file than the one whose card was clicked.
  it("encodes segments the same way the thumbnail route does", () => {
    expect(clipHoverURL("back in 5?.mp4")).toBe("/v1/filler/hover/back%20in%205%3F.mp4");
    expect(clipMediaURL("a#b.mp4")).toBe("/v1/filler/media/a%23b.mp4");
    expect(clipMediaURL("a/b/c.mp4")).not.toContain("%2F");
  });

  // ⚠ **`data:` passes through, `http(s):` must NOT.** The pass-through is what lets stories
  // render offline (the visual suite runs against storybook-static with no server); allowing any
  // absolute URL would let a clip path become a resource fetched from an arbitrary origin — a
  // beacon that leaks who is browsing the catalog and when.
  it("passes a data URI through but never an absolute http URL", () => {
    const inline = "data:video/webm;base64,AAAA";
    expect(clipHoverURL(inline)).toBe(inline);
    expect(clipMediaURL(inline)).toBe(inline);

    // ⚠ THE assertion: an absolute URL is treated as a PATH and prefixed, never returned as-is.
    // It comes back as nonsense that 404s, which is the correct outcome — the point is only that
    // the browser never fetches `evil.example`.
    //
    // Note the shape: the scheme's `:` is encoded but the slashes are NOT, because encoding is
    // per-segment (whole-path encoding would escape real separators to %2F and break the route's
    // `{path...}` wildcard — see the thumbnail tests above).
    const remote = "https://evil.example/track.webp";
    expect(clipHoverURL(remote)).toBe("/v1/filler/hover/https%3A//evil.example/track.webp");
    expect(clipHoverURL(remote)).not.toBe(remote);
    expect(clipHoverURL(remote).startsWith("/v1/filler/hover/")).toBe(true);
  });
});
