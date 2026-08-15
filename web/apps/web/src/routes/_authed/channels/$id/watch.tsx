import * as channelsApi from "@loomarr/api/endpoints/channels";
import { unwrap } from "@loomarr/api/unwrap";
import { createFileRoute } from "@tanstack/react-router";
import { useCallback } from "react";
import { ChannelWatch } from "@/channels/channel-watch";
import { useChannelTuner } from "@/channels/use-channel-tuner";
import { useChannelDetail } from "./-channel-detail-context";

// WATCH — play the channel live in the browser (§9.1, V46). A VIEWER surface (like Overview): a
// member reaches it too, so it is not gated on isAdmin here. The channel-level audio/subtitle
// pickers inside gate their own editability on isAdmin — a member sees the values, an admin
// changes them.
const WatchScreen = () => {
  const { id } = Route.useParams();
  const navigate = Route.useNavigate();
  const { channel, isAdmin, savePolicy } = useChannelDetail();
  const channels = channelsApi.useListChannels({ query: { staleTime: 30_000 } });
  const nowNext = channelsApi.useChannelsNowNext({ query: { staleTime: 15_000 } });
  const tune = useCallback(
    (target: { id: string }) => {
      void navigate({ to: "/channels/$id/watch", params: { id: target.id }, replace: true });
    },
    [navigate],
  );
  const tuner = useChannelTuner({
    currentId: id,
    channels: unwrap(channels.data)?.channels ?? [],
    nowNext: unwrap(nowNext.data)?.channels ?? [],
    onTune: tune,
  });
  const tunedChannel = tuner.channel ?? channel;
  return (
    <>
      {/* A visually-hidden heading, same as filler.tsx. The Watch surface labels itself visibly through
          the player's own top bar (CH n + channel name) and the "Watch live" poster, none of which is a
          heading; as its own route it still needs a real heading for the page to be provably reachable —
          see the visible-heading reachability check — without adding a caption the mock doesn't have. */}
      <h2 className="sr-only">Watch {tunedChannel.name}</h2>
      <ChannelWatch
        channel={tunedChannel}
        isAdmin={isAdmin}
        onSavePolicy={savePolicy}
        tuner={{
          canSurf: tuner.canSurf,
          requestedChannel: tuner.requestedChannel,
          currentTitle: tuner.currentTitle,
          attempt: tuner.attempt,
          acknowledging: tuner.acknowledging,
          ready: tuner.ready,
          step: tuner.step,
          retry: tuner.retry,
        }}
      />
    </>
  );
};

const Route = createFileRoute("/_authed/channels/$id/watch")({
  component: WatchScreen,
});

export { Route };
