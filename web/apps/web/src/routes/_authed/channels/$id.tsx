import { channelsApi } from "@loomarr/api";
import { useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import { ArrowLeft } from "lucide-react";
import { channelHealth, channelOnAir } from "@/channels";
import { ChannelCard, ErrorState } from "@/components/loomarr";
import { Badge, Button } from "@/components/ui";
import { ChannelPods } from "@/filler";

// Channel detail (§12): the card plus the things that explain WHY it looks the way it
// does — slot fill, the Tunarr link, and the programming policy actually in force
// (programming-design §2), including any relaxations the last reconcile had to apply.
const ChannelDetailScreen = () => {
  const { id } = Route.useParams();
  const queryClient = useQueryClient();
  const channel = channelsApi.useGetChannel(id);

  const reconcile = channelsApi.useReconcileChannel({
    mutation: {
      onSuccess: () => queryClient.invalidateQueries({ queryKey: channelsApi.getGetChannelQueryKey(id) }),
    },
  });

  if (channel.error) {
    return (
      <div className="p-6">
        <ErrorState error={channel.error} onRetry={() => channel.refetch()} />
      </div>
    );
  }
  const ch = channel.data?.status === 200 ? channel.data.data : undefined;
  if (!ch) return <p className="p-6 text-muted-foreground text-sm">Loading channel…</p>;

  const applied = ch.policy?.applied ?? [];

  return (
    <div className="flex h-full flex-col">
      <header className="flex items-center gap-3 border-border border-b px-6 py-4">
        <Link to="/channels" className="text-muted-foreground hover:text-foreground">
          <ArrowLeft className="size-4" aria-label="Back to channels" />
        </Link>
        <h1 className="font-semibold text-xl">{ch.name}</h1>
      </header>

      <div className="flex flex-1 flex-col gap-6 overflow-auto p-6">
        <ChannelCard
          number={ch.number}
          name={ch.name}
          onAir={channelOnAir(ch)}
          health={channelHealth(ch)}
          managed
        />

        <section className="flex flex-col gap-2">
          <h2 className="font-semibold text-lg">Lineup</h2>
          <p className="text-muted-foreground text-sm">
            {ch.programCount} of {ch.slotCount} slots have a real program. The rest play filler or flex, so
            the channel never goes to dead air while acquisitions land.
          </p>
          {ch.tunarrId ? (
            <p className="font-mono text-muted-foreground text-xs">Tunarr id {ch.tunarrId}</p>
          ) : (
            <p className="text-muted-foreground text-xs">
              Not pushed to Tunarr yet — the first reconcile creates it.
            </p>
          )}
        </section>

        <section className="flex flex-col gap-2">
          <h2 className="font-semibold text-lg">Programming policy</h2>
          {applied.length > 0 ? (
            <div className="flex flex-col gap-2">
              {/* The relaxation ladder is the honest bit: it says what Loomarr had to
                  loosen to fill the channel, rather than silently producing a lineup
                  that doesn't match the stated intent (programming-design §9). */}
              <p className="text-muted-foreground text-sm">To fill this channel, Loomarr relaxed:</p>
              <div className="flex flex-wrap gap-1.5">
                {applied.map((step) => (
                  // Each relaxation is structured (kind/from/to), so the chip says what
                  // was loosened and by how much — "audience: TV-Y → TV-Y7" — rather
                  // than an opaque label the operator has to decode.
                  <Badge key={`${step.kind}:${step.from}->${step.to}`} variant="caution">
                    {step.kind}: {step.from} → {step.to}
                  </Badge>
                ))}
              </div>
            </div>
          ) : (
            <p className="text-muted-foreground text-sm">
              Running on the channel's stated policy — nothing had to be relaxed.
            </p>
          )}
        </section>

        <section className="flex flex-col gap-2">
          <h2 className="font-semibold text-lg">Commercials</h2>
          <p className="text-muted-foreground text-sm">
            The clip pool Tunarr draws from for this channel's breaks — assembled the same way reconcile does
            it, so this is what actually plays.
          </p>
          <ChannelPods channelId={id} />
        </section>

        <div className="flex items-center gap-2">
          <Button onClick={() => reconcile.mutate({ id })} disabled={reconcile.isPending}>
            {reconcile.isPending ? "Reconciling…" : "Reconcile now"}
          </Button>
        </div>
        {reconcile.error != null && <ErrorState error={reconcile.error} />}
      </div>
    </div>
  );
};

const Route = createFileRoute("/_authed/channels/$id")({
  component: ChannelDetailScreen,
});

export { Route };
