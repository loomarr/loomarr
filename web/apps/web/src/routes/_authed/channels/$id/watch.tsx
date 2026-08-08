import { createFileRoute } from "@tanstack/react-router";
import { ChannelWatch } from "@/channels";
import { useChannelDetail } from "./-channel-detail-context";

// WATCH — play the channel live in the browser (§9.1, V46). A VIEWER surface (like Overview): a
// member reaches it too, so it is not gated on isAdmin here. The channel-level audio/subtitle
// pickers inside gate their own editability on isAdmin — a member sees the values, an admin
// changes them.
const WatchScreen = () => {
  const { channel, isAdmin, savePolicy } = useChannelDetail();
  return <ChannelWatch channel={channel} isAdmin={isAdmin} onSavePolicy={savePolicy} />;
};

const Route = createFileRoute("/_authed/channels/$id/watch")({
  component: WatchScreen,
});

export { Route };
