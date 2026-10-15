import { createFileRoute } from "@tanstack/react-router";
import { AdminOnly } from "@/auth/admin-only";
import { useChannelDetail } from "./-channel-detail-context";
import { ChannelProgramming } from "./-channel-programming";

// Programming — admin-only (a member gets AdminOnly's no-access state, #1977). Folds the old
// Lineup + Programming-rules + Refine tabs into one surface (P7).
const ProgrammingScreen = () => {
  const { id, channel: ch, savePolicy, invalidate } = useChannelDetail();

  return (
    <AdminOnly>
      <ChannelProgramming
        channelId={id}
        channelName={ch.name}
        lineup={ch.lineup ?? []}
        policy={ch.policy}
        onPolicyChange={savePolicy}
        strategy={ch.strategy}
        intentRef={ch.intentRef}
        onRefined={invalidate}
      />
    </AdminOnly>
  );
};

const Route = createFileRoute("/_authed/channels/$id/programming")({
  component: ProgrammingScreen,
});

export { Route };
