import { describe, expect, it } from "vitest";
import { clipThumbURL } from "./clip-thumb";

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
