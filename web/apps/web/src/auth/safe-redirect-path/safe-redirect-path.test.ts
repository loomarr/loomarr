import { describe, expect, it } from "vitest";
import { safeRedirectPath } from "./safe-redirect-path";

// The hostile cases are the point of this function, so they come first and are named for the
// attack rather than the input. Every one of these reached `window.location.replace()` before
// this validator existed — `/login?redirect=<value>` on an already-signed-in browser.
describe("safeRedirectPath: refuses anything that leaves the app", () => {
  it("refuses an absolute http(s) URL", () => {
    expect(safeRedirectPath("https://evil.example")).toBeUndefined();
    expect(safeRedirectPath("http://evil.example/guide")).toBeUndefined();
  });

  it("refuses a protocol-relative URL, which inherits the current scheme", () => {
    expect(safeRedirectPath("//evil.example")).toBeUndefined();
    expect(safeRedirectPath("//evil.example/guide")).toBeUndefined();
  });

  // ⚠ THE ONE A PREFIX CHECK MISSES. Per the WHATWG URL spec a browser resolving a
  // special-scheme URL treats `\` as `/`, so this navigates to https://evil.test while passing
  // startsWith("/") && !startsWith("//") && !includes("://"). Go's safeReturnPath carries the
  // same case for the same reason.
  it("refuses a backslash that a browser reads as a slash", () => {
    expect(safeRedirectPath("/\\evil.test")).toBeUndefined();
  });

  // ⚠ AND the narrow fix is not enough: rejecting only a LEADING `/\` still admits this, which
  // is why the character is refused anywhere in the value.
  it("refuses a backslash that is not leading", () => {
    expect(safeRedirectPath("/\\/evil.test")).toBeUndefined();
    expect(safeRedirectPath("/guide\\@evil.test")).toBeUndefined();
  });

  it("refuses a non-http scheme", () => {
    expect(safeRedirectPath("javascript:alert(1)")).toBeUndefined();
    expect(safeRedirectPath("data:text/html,<script>")).toBeUndefined();
  });

  it("refuses a bare path with no leading slash, which would resolve relative to the page", () => {
    expect(safeRedirectPath("guide")).toBeUndefined();
    expect(safeRedirectPath("evil.example")).toBeUndefined();
  });

  it("refuses absent, empty and non-string values", () => {
    expect(safeRedirectPath(undefined)).toBeUndefined();
    expect(safeRedirectPath("")).toBeUndefined();
    // validateSearch hands through whatever was in the URL, so a non-string is reachable.
    expect(safeRedirectPath(42)).toBeUndefined();
    expect(safeRedirectPath({ toString: () => "/guide" })).toBeUndefined();
  });
});

// The whole reason the param exists: a deep link survives the sign-in round trip. A validator
// that rejected these would be secure and useless.
describe("safeRedirectPath: preserves a real destination", () => {
  it("keeps a plain path", () => {
    expect(safeRedirectPath("/guide")).toBe("/guide");
  });

  it("keeps the reported deep link, sub-tab and all", () => {
    expect(safeRedirectPath("/channels/ch_afeb13f3bb12c7ad/filler")).toBe(
      "/channels/ch_afeb13f3bb12c7ad/filler",
    );
  });

  it("keeps the query string, which carries channel-detail sections", () => {
    expect(safeRedirectPath("/channels/ch_1?section=programming")).toBe("/channels/ch_1?section=programming");
  });

  it("keeps a hash", () => {
    expect(safeRedirectPath("/help#troubleshooting")).toBe("/help#troubleshooting");
  });

  // An @ in the path is not a credential separator once the value is known to be a path, and
  // refusing it would break any future route carrying an email-ish segment.
  it("keeps an @ inside the path", () => {
    expect(safeRedirectPath("/people/a@b.test")).toBe("/people/a@b.test");
  });
});
