import { createFileRoute } from "@tanstack/react-router";
import { ChannelWatch } from "@/channels";
import { useChannelDetail } from "./-channel-detail-context";

// WATCH — play the channel live in the browser (§9.1, V46). A VIEWER surface (like Overview): a
// member reaches it too, so it is not gated on isAdmin here. The channel-level audio/subtitle
// pickers inside gate their own editability on isAdmin — a member sees the values, an admin
// changes them.
const WatchScreen = () => {
  const { channel, isAdmin, savePolicy } = useChannelDetail();
  return (
    <>
      {/* A visually-hidden heading, same as filler.tsx. The Watch surface labels itself visibly through
          the player's own top bar (CH n + channel name) and the "Watch live" poster, none of which is a
          heading; as its own route it still needs a real heading for the page to be provably reachable —
          see the visible-heading reachability check — without adding a caption the mock doesn't have. */}
      <h2 className="sr-only">Watch {channel.name}</h2>
      <ChannelWatch channel={channel} isAdmin={isAdmin} onSavePolicy={savePolicy} />
    </>
  );
};

const Route = createFileRoute("/_authed/channels/$id/watch")({
  component: WatchScreen,
});

export { Route };
