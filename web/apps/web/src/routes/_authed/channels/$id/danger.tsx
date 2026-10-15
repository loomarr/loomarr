import { createFileRoute } from "@tanstack/react-router";
import { AdminOnly } from "@/auth/admin-only";
import { ChannelDangerZone } from "@/components/loomarr/channels/channel-danger-zone";
import { useChannelDetail } from "./-channel-detail-context";

// Danger zone — admin-only (a member gets AdminOnly's no-access state, #1977). Pause/resume and
// delete.
const DangerScreen = () => {
  const { channel, saving, deleting, save, onDelete } = useChannelDetail();

  return (
    <AdminOnly>
      <ChannelDangerZone
        channelName={channel.name}
        status={channel.status}
        busy={saving || deleting}
        onPause={() => save({ status: "paused" })}
        onResume={() => save({ status: "building" })}
        onDelete={onDelete}
      />
    </AdminOnly>
  );
};

const Route = createFileRoute("/_authed/channels/$id/danger")({
  component: DangerScreen,
});

export { Route };
