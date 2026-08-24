// The guide's request window lives in the shared client domain. This adapter keeps the existing
// web import stable while mobile and TV consume the same behavior through @loomarr/core/guide.
//
// GET /v1/guide is cached under a react-query key of `['/v1/guide', {from, to}]` — the exact
// millisecond values. That makes prefetching the route on hover useless unless the prefetch
// computes the SAME numbers the component will a moment later: `Date.now()` moves between the
// hover and the click, so an unquantised window produces a different key, warms an entry nobody
// reads, and still shows a spinner on arrival.
//
// So TODAY's window start is quantised to a minute. The reader loses nothing — the axis already
// only claims minute resolution, and the now-line is drawn from its own live clock rather than
// from the window — and in exchange a hover and the click that follows it address one cache
// entry. It also lines up with the server's own arranged-cycle bucket, which is a minute for the
// same reason: two requests a few seconds apart should not each pay for a fresh arrangement.
//
// The other branches (an explicit start hour, a non-today day) are already derived from midnight
// and so are stable without help.

export type { GuideWindowArgs } from "@loomarr/core/guide";
export {
  DEFAULT_WINDOW_MINUTES,
  defaultGuideWindow,
  GUIDE_BUCKET_MS,
  guideWindow,
  LOOKBACK_MINUTES,
} from "@loomarr/core/guide";
