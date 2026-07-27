// The guide's request window, in ONE place so the page and its prefetch cannot disagree.
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

// GUIDE_BUCKET_MS mirrors the server's cycleBucket (internal/app/cyclecache.go). They are not
// required to be equal — each is correct on its own — but keeping them the same means a client
// window and the arrangement it lands on expire together rather than at staggered times.
const GUIDE_BUCKET_MS = 60_000;

// How much of the window sits behind "now" on TODAY. A guide opens on the present, but a
// programme already in progress needs its real start on screen or it appears to begin the moment
// the page was opened.
const LOOKBACK_MINUTES = 30;

const startOfDay = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();

type GuideWindowArgs = {
  /** The clock the window is computed against — the page's pinned `mountedAt`, or Date.now() for a prefetch. */
  at: number;
  dayOffset: number;
  windowMinutes: number;
  hourShift: number;
  startHour: number | null;
};

// guideWindow resolves the {from, to} the guide requests.
//
// Today opens on the present (minus a little lookback); any other day starts at midnight, because
// "what is on at 3pm on Thursday" is the question a future day is asked. An explicit START hour
// overrides both, and the ‹ › stepper shifts whatever that resolved to.
const guideWindow = ({
  at,
  dayOffset,
  windowMinutes,
  hourShift,
  startHour,
}: GuideWindowArgs): { from: number; to: number } => {
  const span = windowMinutes * 60_000;
  const shift = hourShift * 3_600_000;
  const midnight = startOfDay(new Date(at)) + dayOffset * 86_400_000;

  if (startHour !== null) {
    const start = midnight + startHour * 3_600_000 + shift;
    return { from: start, to: start + span };
  }
  if (dayOffset === 0) {
    // Quantised — see the note at the top. Floor rather than round so the window never starts in
    // the future, which would hide a programme that is airing right now.
    const start = Math.floor((at - LOOKBACK_MINUTES * 60_000 + shift) / GUIDE_BUCKET_MS) * GUIDE_BUCKET_MS;
    return { from: start, to: start + span };
  }
  return { from: midnight + shift, to: midnight + shift + span };
};

// The window the guide opens on by default — what a prefetch should warm, since an operator
// clicking "Guide" in the nav lands on exactly this.
const DEFAULT_WINDOW_MINUTES = 240;

const defaultGuideWindow = (at: number) =>
  guideWindow({ at, dayOffset: 0, windowMinutes: DEFAULT_WINDOW_MINUTES, hourShift: 0, startHour: null });

export type { GuideWindowArgs };
export { DEFAULT_WINDOW_MINUTES, defaultGuideWindow, GUIDE_BUCKET_MS, guideWindow, LOOKBACK_MINUTES };
