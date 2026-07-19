import { channelsApi } from "@loomarr/api";
import { pluralize } from "@loomarr/core";
import { EmptyState, ErrorState, PodTimeline } from "@/components/loomarr";
import type { ChannelPodsProps } from "./channel-pods.type";

// ChannelPods — §12's "preview a channel's pods". It answers the question an operator
// cannot otherwise answer without watching the channel: what commercials will this
// actually play, and if they look wrong, why?
//
// The preview runs the SAME assembler and seed as reconcile, so what it shows is what
// the channel gets. The match level is the diagnostic: `exact` means era and audience
// both matched; anything else names how far down the §10 fallback ladder assembly went.
const ChannelPods = ({ channelId, className }: ChannelPodsProps) => {
  const pods = channelsApi.usePreviewChannelPods(channelId);

  if (pods.error) return <ErrorState error={pods.error} onRetry={() => pods.refetch()} />;
  if (pods.isLoading) return <p className="text-muted-foreground text-sm">Assembling preview…</p>;

  const body = pods.data?.status === 200 ? pods.data.data : undefined;
  const entries = body?.entries ?? [];

  if (entries.length === 0) {
    return (
      <EmptyState
        title="No commercials for this channel"
        description="The filler catalog is empty, so breaks play as flex. Add clips under Filler, then run Sync."
      />
    );
  }

  return (
    <div className={className}>
      <PodTimeline entries={entries} matchLevel={body?.matchLevel} />
      <p className="mt-2 text-muted-foreground text-sm">
        {`${pluralize(entries.length, "clip")} in this channel's pool. Tunarr draws from it for every break.`}
      </p>
      {body?.matchLevel === "bumper_card" && (
        <p className="mt-1 text-onair-300 text-sm">
          Nothing in the catalog matched, so breaks fall back to the bumper card. Tag some commercials with an
          era and audience to fill them.
        </p>
      )}
    </div>
  );
};

export { ChannelPods };
