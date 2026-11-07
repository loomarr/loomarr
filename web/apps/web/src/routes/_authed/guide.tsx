import * as channelsApi from "@loomarr/api/endpoints/channels";
import { createFileRoute, redirect } from "@tanstack/react-router";
import { GuidePage } from "@/channels/guide-page";
import { defaultGuideWindow } from "@/channels/guide-window";
import { isOriginating, parseOriginationSearch } from "@/suggest/origination-flow";

// The channels surface (§12) — headed "Channels", it is both the cross-channel time grid and
// the app's one origination door. Readable by any authenticated user: the guide is
// viewer-facing, and GET /v1/guide is likewise not admin-gated.
//
// The search params are origination's state (OriginationSearch), and part of the route
// contract. The wizard's guided first channel hands off with `?intent=` (§13's blank-page
// killer), so the handoff lands on a filled form rather than an empty grid with the operator
// wondering where their template went; Requests links `?new=1` and `?job=`, Home `?manual=1`.
const GuideScreen = () => <GuidePage search={Route.useSearch()} />;

const Route = createFileRoute("/_authed/guide")({
  component: GuideScreen,
  validateSearch: parseOriginationSearch,
  // Arriving with no origination params resumes what this tab was doing: the Job it follows, or
  // a draft kept across the AI/TMDB setup trip. Only on ENTRY: Close stays on the route and must
  // not reopen the panel, and a hover preload must not redirect.
  // The remembered run is read on entry only, and imported then: it validates the saved draft with
  // a zod schema that the first download doesn't otherwise carry.
  beforeLoad: async ({ search, cause }) => {
    if (cause !== "enter" || isOriginating(search)) return;
    const { rememberedOrigination } = await import("@/suggest/remembered-origination");
    const remembered = rememberedOrigination();
    if (remembered) throw redirect({ to: "/guide", search: remembered, replace: true });
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
