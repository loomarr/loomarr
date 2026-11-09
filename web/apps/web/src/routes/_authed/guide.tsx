import * as channelsApi from "@loomarr/api/endpoints/channels";
import { createFileRoute, redirect } from "@tanstack/react-router";
import { GuidePage } from "@/channels/guide-page";
import { defaultGuideWindow } from "@/channels/guide-window";
import { legacyGuideOrigination } from "@/suggest/origination-flow";

// The channels surface (§12) — headed "Channels", the cross-channel time grid. Readable by any
// authenticated user: the guide is viewer-facing, and GET /v1/guide is likewise not admin-gated.
//
// Channel origination used to live here, in this route's search params (`/guide?new=1`, `?job=`,
// `?manual=1`, `?intent=`). It has its own route now (`/requests/new`), so an old URL of that kind
// is sent there with its params, rather than opening a grid it no longer answers to.
const GuideScreen = () => <GuidePage />;

const Route = createFileRoute("/_authed/guide")({
  component: GuideScreen,
  // The raw search stays raw, as a legacy URL is read in `beforeLoad`. The page reads only `view`:
  // `?view=grid` is the full grid, and anything else the Now/Next list it opens on (#2787), so the
  // grid survives a reload and a link can open it.
  validateSearch: (search: Record<string, unknown>): Record<string, unknown> & { view?: "grid" } => ({
    ...search,
    view: search.view === "grid" ? "grid" : undefined,
  }),
  beforeLoad: ({ search }) => {
    const origination = legacyGuideOrigination(search);
    if (origination) throw redirect({ to: "/requests/new", search: origination, replace: true });
  },
  // Warm the guide before the component mounts, so arriving from the nav paints rows rather
  // than a spinner. With `defaultPreload: "intent"` this runs on HOVER, which buys the whole
  // round trip: the request is already in flight (or done) by the time the click lands.
  //
  // It works only because the window is QUANTISED (guide-window.ts). The query key is
  // ['/v1/guide', {from, to}] with exact millisecond values, so an unquantised `from` would
  // differ between this loader and the component moments later — warming a key nobody reads.
  //
  // ensureQueryData, not fetchQuery: a cached-and-fresh entry must be reused rather than
  // refetched, or hovering the nav would re-request the guide on every pass of the pointer.
  //
  // NOT awaited, and deliberately so. Awaiting would make the route transition BLOCK on the
  // request — trading a spinner inside the page for a stall on the nav click, which feels worse
  // because nothing on screen acknowledges the press. The page's own `isLoading` still covers
  // the case where the click beats the prefetch, and `retry: false` there keeps a failing guide
  // from being retried twice over.
  loader: ({ context: { queryClient } }) => {
    const { from, to } = defaultGuideWindow(Date.now());
    void queryClient.ensureQueryData(channelsApi.getChannelGuideQueryOptions({ from, to }));
  },
});

export { Route };
