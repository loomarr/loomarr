import { channelsApi } from "@loomarr/api";
import { createFileRoute } from "@tanstack/react-router";
import { defaultGuideWindow, GuidePage } from "@/channels";

// The channels surface (§12) — headed "Channels", it is both the cross-channel time grid and
// the app's one origination door. Readable by any authenticated user: the guide is
// viewer-facing, and GET /v1/guide is likewise not admin-gated.
//
// `?intent=` is part of the route contract. The wizard's guided first channel hands off here
// with a template prefilled (§13's blank-page killer) — the page forwards it into the inline
// describe panel and opens it, so the handoff lands on a filled form rather than an empty grid
// with the operator wondering where their template went.
interface GuideSearch {
  intent?: string;
}

const GuideScreen = () => {
  const { intent } = Route.useSearch();
  return <GuidePage initialIntent={intent} />;
};

const Route = createFileRoute("/_authed/guide")({
  component: GuideScreen,
  validateSearch: (search: Record<string, unknown>): GuideSearch => ({
    intent: typeof search.intent === "string" ? search.intent : undefined,
  }),
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
