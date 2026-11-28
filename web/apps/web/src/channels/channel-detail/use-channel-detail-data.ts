import * as channelsApi from "@loomarr/api/endpoints/channels";
import * as settingsApi from "@loomarr/api/endpoints/settings";
import { SettingKey } from "@loomarr/api/models/settingKey";
import { unwrap } from "@loomarr/api/unwrap";
import { type QueryClient, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { useDocumentTitle } from "@/lib/use-document-title";
import { useRouteContent } from "@/lib/use-route-content";

const FLAVOR_NAMES: Record<string, string> = { emby: "Emby", jellyfin: "Jellyfin" };

// How often the header's minutes-left moves on.
const NOW_TICK_MS = 30_000;

// Every read a change to this channel's lineup makes stale. Live updates need none of this: a
// `channel` SSE frame invalidates every `/v1/channels*` read (§9). This is for callers that change
// the lineup without a frame.
const invalidateChannelDetail = (queryClient: QueryClient, id: string) => {
  for (const queryKey of [
    channelsApi.getGetChannelQueryKey(id),
    channelsApi.getChannelsNowNextQueryKey(),
    // The Overview's "what's on later" strip reads its own per-channel guide endpoint.
    channelsApi.getChannelUpcomingQueryKey(id),
    // The saved pod preview (GET …/pods) lives under its own key, not the channel key, and the
    // Filler section reads the saved break from it.
    channelsApi.getPreviewChannelPodsQueryKey(id),
    // The saved cycle preview also sizes one pass for the Programming page's play-order line.
    channelsApi.getPreviewChannelCycleQueryKey(id),
  ]) {
    void queryClient.invalidateQueries({ queryKey });
  }
};

// The reads behind the channel page's header. The route's loader warms the same queries.
const useChannelDetailData = (id: string, isAdmin: boolean) => {
  const queryClient = useQueryClient();
  const channel = channelsApi.useGetChannel(id);
  // A channel's page has its content once the channel itself has arrived; the header's "On now"
  // line and the section bodies fill in beside it (#3399).
  useRouteContent(channel.data !== undefined);
  // The "Open in …" hand-off names and opens the configured media server. `library.flavor` gives the
  // name (Emby/Jellyfin) and `library.url` its front door, both non-secret settings. An unset URL
  // hides the link rather than leaving it doing nothing. The settings list is admin-only, so a
  // member doesn't ask for it and gets no link.
  const settingsList = settingsApi.useSettingsList({ query: { enabled: isAdmin, retry: false } });
  const settings = unwrap(settingsList.data, (b) => b.settings) ?? [];
  const mediaServerUrl = settings.find((e) => e.key === SettingKey.LibraryURL)?.value || undefined;
  const mediaServerName =
    FLAVOR_NAMES[settings.find((e) => e.key === SettingKey.LibraryFlavor)?.value ?? ""] ??
    "your media server";
  useDocumentTitle(unwrap(channel.data, (b) => b.name));
  // The header's "On now: … · Nm left" reads the shared now/next list; the minutes move on a tick.
  const nowNext = unwrap(channelsApi.useChannelsNowNext({ query: { staleTime: 15_000 } }).data);
  // The programme itself (its series and title) comes from the channel's timeline, which names it
  // the way every surface does; the now/next list has only a title.
  const timeline = unwrap(
    channelsApi.useChannelTimeline(id, undefined, { query: { retry: false, staleTime: 15_000 } }).data,
  );
  const [nowMs, setNowMs] = useState(() => Date.now());
  useEffect(() => {
    const tick = setInterval(() => setNowMs(Date.now()), NOW_TICK_MS);
    return () => clearInterval(tick);
  }, []);

  return {
    channel,
    mediaServerUrl,
    mediaServerName,
    nowNext,
    timeline,
    nowMs,
    invalidate: () => invalidateChannelDetail(queryClient, id),
  };
};

export { invalidateChannelDetail, useChannelDetailData };
