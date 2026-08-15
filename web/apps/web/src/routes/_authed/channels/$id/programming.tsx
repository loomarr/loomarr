import type { UpdateChannelInputBodyStrategy } from "@loomarr/api/models/updateChannelInputBodyStrategy";
import { createFileRoute } from "@tanstack/react-router";
import { useChannelDetail } from "./-channel-detail-context";
import { ChannelProgramming } from "./-channel-programming";

// Programming — admin-only. Folds the old Lineup + Programming-rules + Refine tabs into one
// surface (P7).
const ProgrammingScreen = () => {
  const { id, channel: ch, savePolicy, saveStrategy, invalidate } = useChannelDetail();

  return (
    <ChannelProgramming
      channelId={id}
      channelName={ch.name}
      lineup={ch.lineup ?? []}
      policy={ch.policy}
      onPolicyChange={savePolicy}
      strategy={ch.strategy}
      // Cast because the control hands back a plain string while the generated body types
      // `strategy` as its enum union. The options the Select offers ARE that union
      // (STRATEGY_OPTIONS mirrors ChannelDTO.strategy's enum), so the value is always valid —
      // and the server validates it regardless, which is the check that matters.
      onStrategyChange={(strategy: string) => saveStrategy(strategy as UpdateChannelInputBodyStrategy)}
      intentRef={ch.intentRef}
      onRefined={invalidate}
    />
  );
};

const Route = createFileRoute("/_authed/channels/$id/programming")({
  component: ProgrammingScreen,
});

export { Route };
