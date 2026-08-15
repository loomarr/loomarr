import { createFileRoute } from "@tanstack/react-router";
import { ChannelDangerZone } from "@/components/loomarr/channels/channel-danger-zone";
import { useChannelDetail } from "./-channel-detail-context";

// Danger zone — admin-only. Pause/resume and delete.
const DangerScreen = () => {
  const { channel, saving, deleting, update, onDelete } = useChannelDetail();

  return (
    <ChannelDangerZone
      channelName={channel.name}
      status={channel.status}
      busy={saving || deleting}
      onPause={() => update({ status: "paused" })}
      onResume={() => update({ status: "building" })}
      onDelete={onDelete}
    />
  );
};

const Route = createFileRoute("/_authed/channels/$id/danger")({
  component: DangerScreen,
});

export { Route };
