import { createFileRoute } from "@tanstack/react-router";
import { ChannelFiller } from "@/filler";
import { useChannelDetail } from "./-channel-detail-context";

// Filler — admin-only. This is already a dedicated route, so the sandbox is a normal page rather
// than a permanently-open accordion nested inside one.
const FillerScreen = () => {
  const { id, channel } = useChannelDetail();
  return <ChannelFiller channelId={id} revision={channel.revision} policy={channel.policy} />;
};

const Route = createFileRoute("/_authed/channels/$id/filler")({
  component: FillerScreen,
});

export { Route };
