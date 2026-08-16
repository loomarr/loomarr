import * as channelsApi from "@loomarr/api/endpoints/channels";
import { CHANNEL_TEMPLATES } from "@loomarr/core/templates";
import { createFileRoute } from "@tanstack/react-router";
import { GuidePage } from "@/channels/guide-page";
import { defaultGuideWindow } from "@/channels/guide-window";

// The channels surface (§12) — headed "Channels", it is both the cross-channel time grid and
// the app's one origination door. Readable by any authenticated user: the guide is
// viewer-facing, and GET /v1/guide is likewise not admin-gated.
//
// `?preset=` is the wizard's stable template handoff. `?intent=` remains the legacy free-text
// deep-link contract. Either opens the inline describe panel, but only a known preset id resolves
// the canonical constraints authored with that preset.
interface GuideSearch {
  intent?: string;
  preset?: string;
}

const GuideScreen = () => {
  const { intent, preset } = Route.useSearch();
  const template = CHANNEL_TEMPLATES.find((candidate) => candidate.id === preset);
  const initialIntent = template?.intent ?? (intent ? { description: intent } : undefined);
  return <GuidePage initialIntent={initialIntent} />;
};

const Route = createFileRoute("/_authed/guide")({
  component: GuideScreen,
  validateSearch: (search: Record<string, unknown>): GuideSearch => ({
    intent: typeof search.intent === "string" ? search.intent : undefined,
    preset: typeof search.preset === "string" ? search.preset : undefined,
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
