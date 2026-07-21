import { channelsApi } from "@loomarr/api";
import { formatEpgTime } from "@loomarr/core";
import { useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import { RefreshCw } from "lucide-react";
import { channelHealth, channelOnAir } from "@/channels";
import { ChannelCard, EmptyState, ErrorState } from "@/components/loomarr";
import { Button } from "@/components/ui";

// Channels (§12) — the "is my TV working" screen. Everything here is Loomarr's own state
// except now/next, which is Tunarr's (§6): one call serves every card, so the list costs
// one upstream request no matter how many channels exist. Both queries live-invalidate on
// `channel` SSE frames, so a reconcile finishing elsewhere updates this page.
const ChannelsScreen = () => {
  const queryClient = useQueryClient();
  const channels = channelsApi.useListChannels();
  const nowNext = channelsApi.useChannelsNowNext({ query: { retry: false } });

  const reconcile = channelsApi.useReconcileChannel({
    mutation: {
      onSuccess: () => queryClient.invalidateQueries({ queryKey: channelsApi.getListChannelsQueryKey() }),
    },
  });

  const rows = channels.data?.status === 200 ? (channels.data.data.channels ?? []) : [];
  // Absent now/next is normal (no Tunarr yet, guide not generated) — the card just omits
  // the strip rather than the page failing.
  const byChannel = new Map(
    (nowNext.data?.status === 200 ? (nowNext.data.data.channels ?? []) : []).map((n) => [n.channelId, n]),
  );

  if (channels.error) {
    return (
      <div className="p-6">
        <ErrorState error={channels.error} onRetry={() => channels.refetch()} />
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col">
      <header className="flex items-center justify-between border-border border-b px-6 py-4">
        <div>
          <h1 className="font-semibold text-xl">Channels</h1>
          <p className="mt-1 text-muted-foreground text-sm">
            Every channel Loomarr manages, and what's on right now.
          </p>
        </div>
        <Button
          variant="outline"
          onClick={() => {
            void channels.refetch();
            void nowNext.refetch();
          }}
          disabled={channels.isFetching}
        >
          <RefreshCw className={channels.isFetching ? "animate-spin" : undefined} aria-hidden />
          Refresh
        </Button>
      </header>

      <div className="flex-1 overflow-auto p-6">
        {rows.length === 0 && !channels.isLoading ? (
          // Every list gets exactly one next action (§6) — and the only way to make a
          // channel is to describe one.
          <EmptyState
            title="Dead air"
            description="No channels yet. Describe the channel you want and Loomarr builds the lineup."
            action={{ label: "Suggest a channel" }}
          />
        ) : (
          <ul className="flex flex-col gap-3">
            {rows.map((ch) => {
              const nn = byChannel.get(ch.id);
              return (
                <li key={ch.id} className="flex items-start gap-3">
                  <Link to="/channels/$id" params={{ id: ch.id }} className="min-w-0 flex-1">
                    <ChannelCard
                      number={ch.number}
                      name={ch.name}
                      onAir={channelOnAir(ch)}
                      health={channelHealth(ch)}
                      managed
                      nowNext={
                        nn && {
                          // `until` is what makes the strip read like a guide ("Cheers
                          // ·til 8:00 PM"). NowNextEntry has carried stopMs all along;
                          // omitting it left the affordance rendering nothing.
                          now: nn.now
                            ? { title: nn.now.title, until: formatEpgTime(nn.now.stopMs) }
                            : undefined,
                          next: nn.next ? { title: nn.next.title } : undefined,
                          gap: nn.now?.gap,
                        }
                      }
                    />
                  </Link>
                  <Button
                    variant="outline"
                    onClick={() => reconcile.mutate({ id: ch.id })}
                    disabled={reconcile.isPending}
                    title="Rebuild this channel's lineup from what's available now"
                  >
                    {reconcile.isPending ? "Rebuilding…" : "Rebuild now"}
                  </Button>
                </li>
              );
            })}
          </ul>
        )}
        {reconcile.error != null && <ErrorState error={reconcile.error} className="mt-4" />}
      </div>
    </div>
  );
};

const Route = createFileRoute("/_authed/channels/")({
  component: ChannelsScreen,
});

export { Route };
