// safeRedirectPath keeps only a same-app path, so `?redirect=` can never become an open
// redirect (§11).
//
// It is the frontend mirror of Go's `safeReturnPath` (internal/api/ssoroutes.go:296-309), which
// guards the SSO `next` param. Two validators exist because two different values reach two
// different navigations — the server's `next` and the SPA's `redirect` — and the SPA's was
// unguarded: `/login?redirect=https://evil.example` on an already-signed-in browser became an
// off-site `window.location.replace()`, because the guard's `redirect({ href })` is force-committed
// with `replace: true` by the router.
//
// ⚠ **PARSED, not prefix-matched, and the backslash is why.** The obvious check —
// startsWith("/") && !startsWith("//") && !includes("://") — reads as thorough and lets
// `/\evil.test` straight through: per the WHATWG URL spec a browser resolving a special-scheme URL
// treats `\` as `/`, so that value navigates to https://evil.test while looking like a path. A
// narrower fix (rejecting only a leading `/\`) still admits `/\/evil.test`, which is why the
// character is rejected ANYWHERE in the value, before parsing.
//
// The rejection happens before `new URL` because `new URL` does not apply the backslash rule to
// the *relative* argument in a way we can rely on — the same reason the Go version rejects it
// ahead of `url.Parse`.
//
// Returns `undefined` rather than `""` so callers can fall back with `??` at the point of use;
// every rejected value is indistinguishable from an absent one, which is the intent.

// A base that can never be a real destination. RFC 2606 reserves `.invalid`, so this cannot
// collide with the app's own origin the way `http://localhost` would in dev.
const PROBE_ORIGIN = "http://safe-redirect.invalid";

const safeRedirectPath = (next: unknown): string | undefined => {
  if (typeof next !== "string" || next === "") return undefined;
  if (!next.startsWith("/") || next.startsWith("//")) return undefined;
  if (next.includes("\\")) return undefined;

  let url: URL;
  try {
    url = new URL(next, PROBE_ORIGIN);
  } catch {
    return undefined;
  }

  // Belt and braces: anything carrying its own scheme or host fails the leading-slash test
  // above, so this can only fire if the parser normalised something unexpected. Cheap, and the
  // failure it would catch is the whole point of the function.
  if (url.origin !== PROBE_ORIGIN) return undefined;
  if (!url.pathname.startsWith("/") || url.pathname.startsWith("//")) return undefined;

  return `${url.pathname}${url.search}${url.hash}`;
};

export { safeRedirectPath };
